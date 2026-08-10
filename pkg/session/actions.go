package session

import (
	"fmt"
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

	// ActionLogInfo emits an informational system event
	// It is strictly for audit and debug visibility.
	ActionLogInfo

	// ActionLogStateChange emits a state transition event
	ActionLogStateChange

	// ActionClose instructs the session to immediately terminate the
	// underlying network transport and shut down.
	ActionClose
)

type Action struct {
	Type   ActionType
	Msg    message.Message
	Err    error
	Info   string
	States [2]string
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

// Alter state and produce an action to log the transition
func (engine *Engine) transition(state SessionState) []Action {
	if state != engine.state {
		states := [2]string{engine.state.String(), state.String()}
		engine.state = state
		return []Action{{Type: ActionLogStateChange, States: states}}
	}
	return nil
}

func (engine *Engine) off() []Action {
	if engine.state != SessionClosed {
		logout, _ := engine.Router.Sample("5", spec.SampleOptions{})
		actions := []Action{{Type: ActionSend, Msg: logout}, {Type: ActionClose}}
		return append(actions, engine.transition(SessionClosed)...)
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
		logon, _ := engine.Router.Sample("A", spec.SampleOptions{OptionalFields: map[uint16]any{141: nil}})
		logon.Set(108, fmt.Sprint(engine.heartbeatInt))
		logon.Set(1137, engine.Router.GetDefaultApplVerID())
		logon.Set(141, "Y") // Set ResetSeqNumFlag
		actions := []Action{{Type: ActionSend, Msg: logon}}
		return append(actions, engine.transition(SessionLoggingIn)...)
	}

	return engine.transition(SessionListening)
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
			eventMsg := fmt.Sprintf("No message received in %v, sending TestRequest to counterparty", since.Truncate(time.Second))
			actions = append(actions, Action{Type: ActionLogInfo, Info: eventMsg}, Action{Type: ActionSend, Msg: tr})
			actions = append(actions, engine.transition(SessionStale)...)
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
			actions = append(actions, Action{Type: ActionLogInfo, Info: "Message received, transitioning from Stale to Active."})
			actions = append(actions, engine.transition(SessionActive)...)
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
			actions = append(actions, Action{Type: ActionLogInfo, Info: eventLog})
		}

		// Handle Inbound sequence changes
		if inSeqNum != engine.inSeqNum {
			eventLog := fmt.Sprintf("Silently forced InSeqNum from %d to %d. "+
				"Warning: May cause desync with counterparty.", engine.inSeqNum, inSeqNum)
			actions = append(actions, Action{Type: ActionLogInfo, Info: eventLog})
		}

		// User has manually requested a sequence reset, force heal the session
		if engine.state == SessionOutOfSync {
			actions = append(actions, Action{Type: ActionLogInfo, Info: "Sequence reset requested, healing out of sync session"})
			actions = append(actions, engine.transition(SessionActive)...)
			engine.outOfSyncUntil = 0
		}
	}

	// Reset internal state
	engine.outSeqNum = outSeqNum
	engine.inSeqNum = inSeqNum

	return actions
}

func (engine *Engine) OnDisconnect() []Action {
	return engine.transition(SessionClosed)
}
