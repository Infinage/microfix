package session

import (
	"fmt"
	"slices"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

// If we get logon when we are in SessionNew state we accept it and send a logon back
// If we get a logon when we are in Logging State we were validated and accepted
func (engine *Engine) handleLogon(msg *message.Message) (bool, []Action) {
	// Extract heartbeat interval
	hbIntTag, _ := msg.FindFrom(108, 0)
	hbInt, err := hbIntTag.AsInt()
	if err != nil || hbInt < 1 {
		engine.off()
		logout, _ := engine.Router.Sample("5", spec.SampleOptions{OptionalFields: map[uint16]any{58: nil}})
		logout.Set(58, "Invalid HeartBeatInt [108]")
		return false, []Action{
			{Type: ActionError, Err: fmt.Errorf("Got a Logon with invalid HeartbeatInt [108]: %v", hbIntTag.Value)},
			{Type: ActionSend, Msg: logout},
			{Type: ActionClose},
		}
	}

	// If we were ones to send the logon, we expect heartbeatInt to strictly match
	if engine.state == SessionLoggingIn && hbInt != engine.heartbeatInt {
		engine.off()
		logout, _ := engine.Router.Sample("5", spec.SampleOptions{OptionalFields: map[uint16]any{58: nil}})
		logout.Set(58, "HeartBeatInt [108] mismatch")
		return false, []Action{
			{Type: ActionError, Err: fmt.Errorf("Heartbeat Interval in Logon incorrect, expected %v, got %v", engine.heartbeatInt, hbInt)},
			{Type: ActionSend, Msg: logout},
			{Type: ActionClose},
		}
	}

	var actions []Action

	// If flag set, reset sequence numbers
	inSeqNumTag, _ := msg.FindFrom(34, 0)
	inSeqNum, _ := inSeqNumTag.AsInt()
	if resetSeqNumFlag, _ := msg.Get(141); resetSeqNumFlag == "Y" {
		engine.inSeqNum = 1
		engine.store.Reset() // Remove all message from store

		// Only reset OutSeq if we are the acceptor (receiving logon as 1st msg)
		// Otherwise we are receving logon as a response, outSeqNum is already at 2
		if engine.state == SessionListening {
			engine.outSeqNum = 1
		}
	} else if inSeqNum > engine.inSeqNum {
		// Case where inSeqNum < engine.inSeqNum would be rejected and handled by engine.validate
		// although such a scenario is unlikely, since we do not persist messages across restarts
		engine.inSeqNum = inSeqNum
		eventMsg := fmt.Sprintf("Logon InSeq [34] higher than expected, force set to %d", inSeqNum)
		actions = append(actions, Action{Type: ActionLogInfo, Info: eventMsg})
	}

	// We are SessionListening and Counterparty sends a logon, accept and send back a logon
	if engine.state == SessionListening {
		// Update negotiated heartbeat from input message
		engine.heartbeatInt = hbInt

		// Extract DefaultApplVerID from logon, gauranteed to pass
		// since validate would have already caught it
		applVerID, _ := msg.Get(1137)
		engine.Router.SetDefaultApplVerID(applVerID)

		// Build a logon response back and add heartbeat interval + applVerID if applicable
		logon, _ := engine.Router.Sample("A", spec.SampleOptions{})
		logon.Set(108, fmt.Sprint(hbInt))
		logon.Set(1137, applVerID)

		// Send logon request back and set state to active
		actions = append(actions,
			Action{Type: ActionLogInfo, Info: "Logon request received, transitioning from Listening to Active"},
			Action{Type: ActionSend, Msg: logon})
	}

	// If all good, we proceed to next stage
	actions = append(actions, engine.transition(SessionActive)...)
	return true, actions
}

func (engine *Engine) handleResend(msg *message.Message) []Action {
	var actions []Action
	var beginSeq, endSeq int64
	var err error

	// We can ignore the error here since we would have
	// already caught it during validation
	seqNoField, _ := msg.FindFrom(34, 0)
	seqNo, _ := seqNoField.AsInt()

	beginSeqNoField, _ := msg.FindFrom(7, 0)
	if beginSeq, err = beginSeqNoField.AsInt(); err != nil || beginSeq <= 0 {
		err := &RejectError{RefSeqNum: seqNo, Text: "Invalid BeginSeqNo [7] value"}
		actions = append(actions, Action{Type: ActionError, Err: err}, engine.reject(err))
		return actions
	}

	endSeqNoField, _ := msg.FindFrom(16, 0)
	if endSeq, err = endSeqNoField.AsInt(); err != nil {
		err := &RejectError{RefSeqNum: seqNo, Text: "Invalid EndSeqNo [16] value"}
		actions = append(actions, Action{Type: ActionError, Err: err}, engine.reject(err))
		return actions
	}

	// outSeqNum represents the seqno of next message to sent
	// last message sent to client would have outSeqNum - 1
	if endSeq == 0 || endSeq > engine.outSeqNum-1 {
		endSeq = engine.outSeqNum - 1
	}

	// Build seq reset msg once and reuse whenever needed
	optFields := map[uint16]any{43: nil, 123: nil}
	seqResetTemplate, _ := engine.Router.Sample("4", spec.SampleOptions{OptionalFields: optFields})
	seqResetTemplate.Set(123, "Y") // GapFillFlag

	// Tracking last seq that was sent out
	prevSeqNo := beginSeq - 1
	for _, replayEntry := range engine.store.Fetch(beginSeq, endSeq) {
		// Send a reset sequence whenever there is a gap
		if replayEntry.seqNo > prevSeqNo+1 {
			seqReset := slices.Clone(seqResetTemplate)
			seqReset.Set(34, fmt.Sprint(prevSeqNo+1)) // outSeqNum field update is bypassed on finalize for reset requests
			seqReset.Set(36, fmt.Sprint(replayEntry.seqNo))
			seqReset.Set(43, "Y")
			actions = append(actions, Action{Type: ActionSend, Msg: seqReset})
		}

		// Insert PossDupFlag, OrigSendingTime after MsgSeqNo (> 3 should be okay)
		replay := slices.Clone(replayEntry.Msg)
		origSendingTime, _ := replay.Get(52)
		replay.Insert(6, message.Field{Tag: 43, Value: "Y"})
		replay.Insert(7, message.Field{Tag: 122, Value: origSendingTime})
		actions = append(actions, Action{Type: ActionSend, Msg: replay})
		prevSeqNo = replayEntry.seqNo
	}

	// If last msg sent is behind the requested seqno, send a final gapfill
	if prevSeqNo < endSeq {
		seqReset := slices.Clone(seqResetTemplate)
		seqReset.Set(34, fmt.Sprint(prevSeqNo+1))
		seqReset.Set(36, fmt.Sprint(endSeq+1))
		seqReset.Set(43, "Y")
		actions = append(actions, Action{Type: ActionSend, Msg: seqReset})
	}

	return actions
}

func (engine *Engine) handleOutSyncMessage(msg *message.Message) (bool, []Action) {
	msgType, _ := msg.Get(35)
	inSeqNumTag, _ := msg.FindFrom(34, 0)
	inSeqNum, _ := inSeqNumTag.AsInt()

	// If ResendRequest, bypass OutOfSync state (disregarding inSeqNum > expected)
	if msgType == "2" {
		return false, engine.handleResend(msg)
	}

	// If hard Sequence Reset (123=N), pass through std handler to reset and self heal
	gapFillFlag, _ := msg.Get(123)
	if msgType == "4" && gapFillFlag == "N" {
		accepted, actions := engine.handleAppMessage(msg)
		actions = append(actions, Action{Type: ActionLogInfo, Info: "Hard reset received, healing OutOfSync session."})
		actions = append(actions, engine.transition(SessionActive)...)
		return accepted, actions
	}

	// Drop any message that don't match our inSeqNum
	if inSeqNum != engine.inSeqNum {
		return false, []Action{
			{Type: ActionLogInfo, Info: fmt.Sprintf("OutSync: Dropped msg %d, still waiting for MsgSeq# %d", inSeqNum, engine.inSeqNum)},
		}
	}

	// Process replayed message if it matches expected InSeqNum
	accepted, actions := engine.handleAppMessage(msg)
	if engine.inSeqNum+1 >= engine.outOfSyncUntil {
		actions = append(actions, Action{Type: ActionLogInfo, Info: "Sequence gap resolved, transitioning from OutOfSync to Active."})
		actions = append(actions, engine.transition(SessionActive)...)
	}

	return accepted, actions
}

func (engine *Engine) handleAppMessage(msg *message.Message) (bool, []Action) {
	// Get the MsgType
	msgType, _ := msg.Get(35)

	// Trigger a resend request (replay), if inSeqNum greater than what we are expecting
	inSeqNumTag, _ := msg.FindFrom(34, 0)
	gapFillFlag, _ := msg.Get(123)
	isHardReset := msgType == "4" && gapFillFlag == "N"
	if inSeqNum, _ := inSeqNumTag.AsInt(); !isHardReset && inSeqNum > engine.inSeqNum {
		engine.outOfSyncUntil = inSeqNum
		resend, _ := engine.Router.Sample("2", spec.SampleOptions{})
		resend.Set(7, fmt.Sprint(engine.inSeqNum))
		resend.Set(16, "0") // 0 means infinity in FIX Resend requests
		actions := []Action{
			{Type: ActionError, Err: fmt.Errorf("Expected InSeqNum [34] %d, got %d, triggering resend request.", engine.inSeqNum, inSeqNum)},
			{Type: ActionSend, Msg: resend},
			{Type: ActionLogInfo, Info: fmt.Sprintf("Transitioning to OutOfSync, waiting for MsgSeq# %d", inSeqNum)},
		}
		return false, append(actions, engine.transition(SessionOutOfSync)...)
	}

	// Actions based on message type and struct
	var actions []Action

	switch msgType {
	case "0":
		// If we are stale, we expect the heartbeat to echo our TestReqID
		if engine.state == SessionStale {
			reqID, _ := msg.Get(112)
			if reqID != engine.testReqID {
				errMsg := fmt.Errorf("Expected Heartbeat TestReqID tag [112] to be '%v'", engine.testReqID)
				actions = append(actions, Action{Type: ActionError, Err: errMsg})
			}
		}

	case "1": // Handle Test Request
		hb, _ := engine.Router.Sample("0", spec.SampleOptions{OptionalFields: map[uint16]any{112: nil}})
		if reqId, ok := msg.Get(112); ok {
			hb.Set(112, reqId)
		}
		actions = append(actions, Action{Type: ActionSend, Msg: hb})

	case "2": // Resend request
		actions = append(actions, engine.handleResend(msg)...)

	case "4": // Sequence Reset
		seqNoTag, _ := msg.FindFrom(36, 0)
		val, err := seqNoTag.AsInt()
		if err != nil {
			seqNoField, _ := msg.FindFrom(34, 0)
			seqNo, _ := seqNoField.AsInt()
			err := &RejectError{RefSeqNum: seqNo, Text: "Invalid SeqNo [36] value"}
			actions = append(actions, Action{Type: ActionError, Err: err}, engine.reject(err))
		} else if val < engine.inSeqNum {
			err := &RejectError{
				RefSeqNum: engine.inSeqNum,
				Text:      fmt.Sprintf("NewSeqNo [%d] is lower than expected [%d]", val, engine.inSeqNum),
			}
			actions = append(actions, Action{Type: ActionError, Err: err}, engine.reject(err))
		} else {
			engine.inSeqNum = val
			eventLog := fmt.Sprintf("InSeqNum has been reset to %v", seqNoTag.Value)
			actions = append(actions, Action{Type: ActionLogInfo, Info: eventLog})
		}
		return false, actions

	case "5": // Logout
		actions = append(actions, engine.off()...)

	default: // Passthrough (blocking until session is closed)
		actions = append(actions, Action{Type: ActionDeliver, Msg: *msg})
	}

	return true, actions
}
