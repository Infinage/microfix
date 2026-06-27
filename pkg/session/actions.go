package session

import (
	"fmt"
	"slices"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

type ActionType int

const (
	// ActionSend instructs the session to transmit a FIX message over the network
	// to the connected counterparty.
	ActionSend ActionType = iota

	// ActionDeliver routes a valid, application-level FIX message up the stack
	// to be processed by the user's business logic.
	ActionDeliver

	// ActionError represents a protocol violation or internal fault.
	// It signals an issue that requires logging, but is non-fatal unless
	// accompanied by an ActionClose.
	ActionError

	// ActionLog emits an informational system event or state transition
	// (e.g., "Session transitioning from Active to Stale"). It is strictly for audit
	// and debug visibility.
	ActionLog

	// ActionClose instructs the session to immediately terminate the
	// underlying network transport and shut down.
	ActionClose
)

type Action struct {
	Type  ActionType
	Msg   message.Message
	Err   error
	Event string
}

// Helper to build a Reject ['35=3'] message
func (engine *Engine) reject(err *RejectError) Action {
	rejectMsg, _ := engine.Router.Sample("3", spec.SampleOptions{
		OptionalFields: map[uint16]any{58: nil},
	})
	rejectMsg.Set(45, fmt.Sprint(err.RefSeqNum))
	rejectMsg.Set(58, err.Text)
	return Action{Type: ActionSend, Msg: rejectMsg}
}

func (engine *Engine) off() []Action {
	if engine.state != SessionClosed {
		engine.state = SessionClosed
		logout, _ := engine.Router.Sample("5", spec.SampleOptions{})
		return []Action{{Type: ActionSend, Msg: logout}, {Type: ActionClose}}
	}
	return nil
}

// First event to be called manually by one who initialized engine,
// specifying whether to run engine as a server or as a client
func (engine *Engine) OnStart(isClient bool) []Action {
	now := time.Now()
	engine.lastReadTime = now
	engine.lastWriteTime = now

	if isClient {
		engine.state = SessionLoggingIn
		logon, _ := engine.Router.Sample("A", spec.SampleOptions{OptionalFields: map[uint16]any{141: nil}})
		logon.Set(108, fmt.Sprint(engine.heartbeatInt))
		logon.Set(1137, engine.Router.GetDefaultApplVerID())
		logon.Set(141, "Y") // Set ResetSeqNumFlag
		return []Action{{Type: ActionSend, Msg: logon}}
	}

	engine.state = SessionListening
	return nil
}

// Handle timeouts, track and send heartbeats
func (engine *Engine) OnTick(now time.Time) []Action {

	// Timeout logon requests if we did not receive a logon back
	if engine.state == SessionLoggingIn && now.Sub(engine.lastWriteTime) > 3*time.Second {
		engine.off()
		return []Action{
			{Type: ActionError, Err: fmt.Errorf("Logon timeout")},
			{Type: ActionClose}, // No logout sent
		}
	}

	// Check for outgoing / incoming idle
	var actions []Action
	hbDuration := time.Second * time.Duration(engine.heartbeatInt)

	// Outgoing idle (send heartbeat)
	if now.Sub(engine.lastWriteTime) >= hbDuration {
		hb, _ := engine.Router.Sample("0", spec.SampleOptions{})
		actions = append(actions, Action{Type: ActionSend, Msg: hb})
	}

	// Incoming idle (send test request)
	// Even if session is out of sync, let it stale & sort the staleness first
	if since := now.Sub(engine.lastReadTime); since >= hbDuration {
		if engine.state != SessionStale {
			tr, _ := engine.Router.Sample("1", spec.SampleOptions{})
			tr.Set(112, engine.testReqID)
			eventLog := fmt.Sprintf("No message received in %v. Transitioning from %s to Stale", since.Truncate(time.Second), engine.state)
			actions = append(actions, Action{Type: ActionLog, Event: eventLog}, Action{Type: ActionSend, Msg: tr})
			engine.state = SessionStale
		} else if since >= hbDuration*3 {
			engine.off()
			return []Action{
				{Type: ActionError, Err: fmt.Errorf("Counterparty dead")},
				{Type: ActionClose}, // No logout sent
			}
		}
	}

	return actions
}

// Logic to respond to messages and set states
func (engine *Engine) OnMessage(msg *message.Message, now time.Time) []Action {
	engine.lastReadTime = now

	// Each subhandler returns a list of actions to be returned by this function to session
	var actions []Action

	// Validate the message and logout for non "RejectError"
	if err := engine.validate(msg, now); err != nil {
		actions = append(actions, Action{Type: ActionError, Err: err})
		if rejectErr, ok := err.(*RejectError); ok && engine.state >= SessionActive {
			actions = append(actions, engine.reject(rejectErr))
			engine.inSeqNum++ // Increment seqNo in case of spec validation failure
		} else {
			logout, _ := engine.Router.Sample("5", spec.SampleOptions{OptionalFields: map[uint16]any{58: nil}})
			logout.Set(58, err.Error())
			actions = append(actions, Action{Type: ActionSend, Msg: logout}, Action{Type: ActionClose})
		}
		return actions
	}

	// Every individual handler returns bool if msg is accepted
	// We will update our inSeqNum only when it is accepted
	var msgAccepted bool

	// Get the MessageType from msg object
	msgType, _ := msg.Get(35)

	switch engine.state {
	case SessionListening, SessionLoggingIn:
		if msgType == "A" { // Appropriately handle logon as Server and as Client
			msgAccepted, actions = engine.handleLogon(msg)
		} else {
			rejErr := &RejectError{RefSeqNum: engine.inSeqNum, Text: "First message not a logon"}
			actions = []Action{engine.reject(rejErr)}
			msgAccepted = false
		}

	case SessionStale, SessionActive:
		msgAccepted, actions = engine.handleAppMessage(msg)
		if engine.state == SessionStale {
			engine.state = SessionActive
			actions = append(actions, Action{Type: ActionLog, Event: "Message received. Transitioning from Stale to Active."})
		}

	case SessionOutOfSync:
		msgAccepted, actions = engine.handleOutSyncMessage(msg)
	}

	// Update inbound sequence number
	if msgAccepted {
		engine.inSeqNum++
	}

	return actions
}

func (engine *Engine) OnResetSequence(inSeqNum int64, outSeqNum int64) []Action {
	var actions []Action

	if state := engine.state; state != SessionNew && state != SessionClosed {
		// Handle Outbound sequence changes
		if outSeqNum > engine.outSeqNum {
			// Out seq reset can only go forward per FIX protocol
			seqReset, _ := engine.Router.Sample("4", spec.SampleOptions{OptionalFields: map[uint16]any{123: nil}})
			seqReset.Set(36, fmt.Sprint(outSeqNum))
			seqReset.Set(123, "N")
			actions = append(actions, Action{Type: ActionSend, Msg: seqReset})

		} else if outSeqNum < engine.outSeqNum {
			// Moving backward is not permitted, silently reset anyway for chaos testing
			eventLog := fmt.Sprintf("Silently forced OutSeqNum backward from %d to %d. "+
				"Expect counterparty disconnect on next send.", engine.outSeqNum, outSeqNum)
			actions = append(actions, Action{Type: ActionLog, Event: eventLog})
		}

		// Handle Inbound sequence changes
		if inSeqNum != engine.inSeqNum {
			eventLog := fmt.Sprintf("Silently forced InSeqNum from %d to %d. "+
				"Warning: May cause desync with counterparty.", engine.inSeqNum, inSeqNum)
			actions = append(actions, Action{Type: ActionLog, Event: eventLog})
		}

		// If session state is OutOfSync, heal the session
		if engine.state == SessionOutOfSync {
			actions = append(actions, Action{Type: ActionLog, Event: "Healing out of sync session, transitioning from OutOfSync to Active"})
			engine.state = SessionActive
			engine.outOfSyncUntil = 0
		}
	}

	// Reset internal state
	engine.outSeqNum = outSeqNum
	engine.inSeqNum = inSeqNum

	return actions
}

func (engine *Engine) OnDisconnect() []Action {
	engine.state = SessionClosed
	return nil
}

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
		actions = append(actions, Action{Type: ActionLog, Event: eventMsg})
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
			Action{Type: ActionLog, Event: "Logon request received, transitioning from Listening to Active"},
			Action{Type: ActionSend, Msg: logon})
	}

	// If all good, we proceed to next stage
	engine.state = SessionActive
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
		engine.state = SessionActive
		actions = append(actions, Action{Type: ActionLog, Event: "Hard reset received. Transitioning from OutOfSync to Active."})
		return accepted, actions
	}

	// Drop any message that don't match our inSeqNum
	if inSeqNum != engine.inSeqNum {
		return false, []Action{
			{Type: ActionLog, Event: fmt.Sprintf("OutSync: Dropped msg %d, still waiting for MsgSeq# %d", inSeqNum, engine.inSeqNum)},
		}
	}

	// Process replayed message if it matches expected InSeqNum
	accepted, actions := engine.handleAppMessage(msg)
	if engine.inSeqNum+1 >= engine.outOfSyncUntil {
		engine.state = SessionActive
		actions = append(actions, Action{Type: ActionLog, Event: "Sequence gap resolved, transitioning from OutOfSync to Active."})
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
		eventMsg := fmt.Sprintf("Transitioning from %s to OutOfSync, waiting for MsgSeq# %d", engine.state, inSeqNum)
		engine.state = SessionOutOfSync
		engine.outOfSyncUntil = inSeqNum
		resend, _ := engine.Router.Sample("2", spec.SampleOptions{})
		resend.Set(7, fmt.Sprint(engine.inSeqNum))
		resend.Set(16, "0") // 0 means infinity in FIX Resend requests
		return false, []Action{
			{Type: ActionError, Err: fmt.Errorf("Expected InSeqNum [34] %d, got %d, triggering resend request.", engine.inSeqNum, inSeqNum)},
			{Type: ActionSend, Msg: resend},
			{Type: ActionLog, Event: eventMsg},
		}
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
			actions = append(actions, Action{Type: ActionLog, Event: eventLog})
		}
		return false, actions

	case "5": // Logout
		actions = append(actions, engine.off()...)

	default: // Passthrough (blocking until session is closed)
		actions = append(actions, Action{Type: ActionDeliver, Msg: *msg})
	}

	return true, actions
}
