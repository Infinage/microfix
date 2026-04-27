package session

import (
	"fmt"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

type SessionState int

const (
	SessionDisconnected SessionState = iota
	SessionLoggingIn
	SessionActive
	SessionStale
)

type ActionType int

const (
	ActionSend ActionType = iota
	ActionDeliver
	ActionError
	ActionClose
)

type Engine struct {
	Spec  spec.Spec
	state SessionState

	senderCompID string
	targetCompID string
	heartbeatInt int64
	testReqID    string

	inSeqNum  int64
	outSeqNum int64

	lastWriteTime time.Time
	lastReadTime  time.Time
}

type Action struct {
	Type ActionType
	Msg  message.Message
	Err  error
}

func NewEngine(specPath string, senderCompID string, targetCompID string, heartbeatInt int64) (*Engine, error) {
	if heartbeatInt <= 0 {
		return nil, fmt.Errorf("Heartbeat Interval must be greater than 0")
	}

	// Attempt to load the spec
	sp, err := spec.LoadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to load spec: %v", err)
	}

	// Ensure spec contains the message def for what we will be sending
	for _, msgType := range []string{"0", "1", "2", "3", "4", "5", "A"} {
		if _, err := sp.Sample(msgType, spec.SampleOptions{}); err != nil {
			return nil, fmt.Errorf("Failed to sample message: %v", msgType)
		}
	}

	engine := &Engine{
		Spec:         sp,
		senderCompID: senderCompID,
		targetCompID: targetCompID,
		heartbeatInt: heartbeatInt,
		testReqID:    "MICROFIX",
		inSeqNum:     1,
		outSeqNum:    1,
		state:        SessionDisconnected,
	}

	return engine, nil
}

func (engine *Engine) Off() []Action {
	if engine.state != SessionDisconnected {
		engine.state = SessionDisconnected
		lo, _ := engine.Spec.Sample("5", spec.SampleOptions{})
		return []Action{{Type: ActionSend, Msg: lo}, {Type: ActionClose}}
	}
	return nil
}

func (engine *Engine) OnStart(isClient bool) []Action {
	if isClient {
		engine.state = SessionLoggingIn
		logon, _ := engine.Spec.Sample("A", spec.SampleOptions{})
		logon.Set(108, fmt.Sprint(engine.heartbeatInt))
		return []Action{{Type: ActionSend, Msg: logon}}
	}

	engine.state = SessionDisconnected
	return nil
}

// Feedback that a write action was successful
func (engine *Engine) RecordWrite(now time.Time) {
	engine.lastWriteTime = now
	engine.outSeqNum++
}

// Returns an error if missing: [35, 9, 49, 56, 34, 52, 10]
// Public since 'Session::Send' always finalizes before sending
func (engine *Engine) FinalizeMessage(msg *message.Message, now time.Time) error {
	if !msg.Contains(35, 9, 49, 56, 34, 52, 10) {
		return fmt.Errorf("Missing required tags in OUTBOUND: [35, 9, 49, 56, 34, 52, 10]")
	}

	// Set sender / target compId
	msg.Set(49, engine.senderCompID)
	msg.Set(56, engine.targetCompID)

	// Update OutSeqNum
	msg.Set(34, fmt.Sprint(engine.outSeqNum))

	// Set SendingTime
	msg.Set(52, now.UTC().Format("20060102-15:04:05.000"))

	// Recalculate the bodylen and checksum
	msg.Finalize()

	return nil
}

// Checks for MsgType, InSeqNum, Checksum, BodyLength, TargetCompID, SenderCompID + Spec Validation
func (engine *Engine) validate(msg *message.Message) error {
	msgType, ok := msg.Get(35)
	if !ok {
		return fmt.Errorf("Missing MsgType tag [35]")
	}

	// Validate that message has InSeqNum, set default value guaranteed to fail
	var received int64 = 0
	if inSeqNumTag, seqNoPos := msg.FindFrom(34, 0); seqNoPos != -1 {
		if val, err := inSeqNumTag.AsInt(); err == nil {
			received = val
		}
	}

	// Skip sequence number check if:
	// 1. It's a logon with resetSeqNumFlag set
	// 2. It's a retransmitted message (PossDup)
	possDup, _ := msg.Get(43)
	resetSeqNum, _ := msg.Get(141)
	skipSeqCheck := (msgType == "A" && resetSeqNum == "Y") || possDup == "Y"

	// For values greater than we will trigger resend on `handleAppMessage`
	if !skipSeqCheck && received < engine.inSeqNum {
		return fmt.Errorf("Input sequence number mismatch. Expected %v, got %v", engine.inSeqNum, received)
	}

	// Validate the TargetCompID / SenderCompID: check is swapped
	if senderCompID, _ := msg.Get(49); senderCompID != engine.targetCompID {
		return fmt.Errorf("SenderCompID [49] mismatch, expected '%v' got '%v'", engine.targetCompID, senderCompID)
	}
	if targetCompID, _ := msg.Get(56); targetCompID != engine.senderCompID {
		return fmt.Errorf("TargetCompID [56] mismatch, expected '%v' got '%v'", engine.senderCompID, targetCompID)
	}

	// Validate per input spec
	if ok, obs := engine.Spec.Validate(msg, spec.ValidationBasic); !ok {
		return fmt.Errorf("Message validation failed: %v", obs)
	}

	return nil
}

// Handle timeouts, track and send heartbeats
func (engine *Engine) OnTick(now time.Time) []Action {

	// Timeout logon requests if we did not receive a logon back
	if engine.state == SessionLoggingIn && now.Sub(engine.lastWriteTime) > 3*time.Second {
		engine.Off()
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
		hb, _ := engine.Spec.Sample("0", spec.SampleOptions{})
		actions = append(actions, Action{Type: ActionSend, Msg: hb})
	}

	// Incoming idle (send test request)
	if since := now.Sub(engine.lastReadTime); since >= hbDuration {
		if engine.state != SessionStale {
			tr, _ := engine.Spec.Sample("1", spec.SampleOptions{})
			tr.Set(112, engine.testReqID)
			engine.state = SessionStale
			actions = append(actions, Action{Type: ActionSend, Msg: tr})
		} else if since >= hbDuration*3 {
			engine.Off()
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

	// Validate the message and return its MsgType
	if err := engine.validate(msg); err != nil {
		return []Action{{Type: ActionError, Err: err}}
	}

	// Every individual handler returns bool if msg is accepted
	// We will update our inSeqNum only when it is accepted
	var msgAccepted bool

	// Each subhandler returns a list of actions to be returned by this function to session
	var actions []Action

	// Get the MessageType from msg object
	msgType, _ := msg.Get(35)

	switch engine.state {
	case SessionDisconnected, SessionLoggingIn:
		if msgType == "A" { // Appropriately handle logon as Server and as Client
			msgAccepted, actions = engine.handleLogon(msg)
		}

	case SessionActive:
		msgAccepted, actions = engine.handleAppMessage(msg)

	case SessionStale:
		if msgType == "0" {
			msgAccepted, actions = engine.handleStaleHeartbeat(msg)
		}
	}

	// Update inbound sequence number
	if msgAccepted {
		engine.inSeqNum++
	}

	return actions
}

func (engine *Engine) handleStaleHeartbeat(msg *message.Message) (bool, []Action) {
	var actions []Action
	reqID, _ := msg.Get(112)
	if reqID != engine.testReqID { // Log warn but continue
		actions = append(actions, Action{Type: ActionError, Err: fmt.Errorf("Expected Heartbeat TestReqID tag [112] to %v", engine.testReqID)})
	}
	engine.state = SessionActive
	return true, actions
}

// If we get logon when we are in Disconnected state we accept it and send a logon back
// If we get a logon when we are in Logging State we were validated and accepted
func (engine *Engine) handleLogon(msg *message.Message) (bool, []Action) {
	// Extract heartbeat interval
	hbIntTag, _ := msg.FindFrom(108, 0)
	hbInt, err := hbIntTag.AsInt()
	if err != nil || hbInt < 1 {
		engine.Off()
		lo, _ := engine.Spec.Sample("5", spec.SampleOptions{OptionalFields: map[uint16]any{58: nil}})
		lo.Set(58, "Invalid HeartBeatInt [108]")
		return false, []Action{
			{Type: ActionError, Err: fmt.Errorf("Got a Logon with invalid HeartbeatInt [108]: %v", hbIntTag.Value)},
			{Type: ActionSend, Msg: lo},
			{Type: ActionClose},
		}
	}

	// If we were ones to send the logon, we expect heartbeatInt to strictly match
	if engine.state == SessionLoggingIn && hbInt != engine.heartbeatInt {
		engine.Off()
		lo, _ := engine.Spec.Sample("5", spec.SampleOptions{OptionalFields: map[uint16]any{58: nil}})
		lo.Set(58, "HeartBeatInt [108] mismatch")
		return false, []Action{
			{Type: ActionError, Err: fmt.Errorf("Heartbeat Interval in Logon incorrect, expected %v, got %v", engine.heartbeatInt, hbInt)},
			{Type: ActionSend, Msg: lo},
			{Type: ActionClose},
		}
	}

	var actions []Action

	// If flag set, reset sequence numbers
	if resetSeqNumFlag, _ := msg.Get(141); resetSeqNumFlag == "Y" {
		engine.inSeqNum, engine.outSeqNum = 1, 1
	}

	// We are disconnected and Counterparty sends a logon, accept and send back a logon
	if engine.state == SessionDisconnected {
		// Update negotiated heartbeat from input message
		engine.heartbeatInt = hbInt

		// Build a logon response back and add heartbeat interval
		lo, _ := engine.Spec.Sample("A", spec.SampleOptions{})
		lo.Set(108, fmt.Sprint(hbInt))

		// Send logon request back and set state to active
		actions = append(actions, Action{Type: ActionSend, Msg: lo})
	}

	// If all good, we proceed to next stage
	engine.state = SessionActive
	return true, actions
}

func (engine *Engine) handleAppMessage(msg *message.Message) (bool, []Action) {
	// Get the MsgType
	msgType, _ := msg.Get(35)

	// Trigger a resend request (replay), if inSeqNum greater what we are expecting
	inSeqNumTag, _ := msg.FindFrom(34, 0)
	if inSeqNum, _ := inSeqNumTag.AsInt(); inSeqNum > engine.inSeqNum {
		resend, _ := engine.Spec.Sample("2", spec.SampleOptions{})
		resend.Set(7, fmt.Sprintf("%d", engine.inSeqNum))
		resend.Set(16, fmt.Sprintf("%d", inSeqNum-1))
		return false, []Action{
			{Type: ActionSend, Msg: resend},
			{Type: ActionError, Err: fmt.Errorf("Expected InSeqNum [34] %v, got %v, triggered resend request.", engine.inSeqNum, inSeqNum)},
		}
	}

	// Actions based on message type and struct
	var actions []Action

	switch msgType {
	case "0": // Already updated InSeqNum, noop

	case "1": // Handle Test Request
		hb, _ := engine.Spec.Sample("0", spec.SampleOptions{OptionalFields: map[uint16]any{112: nil}})
		if reqId, ok := msg.Get(112); ok {
			hb.Set(112, reqId)
		}
		actions = append(actions, Action{Type: ActionSend, Msg: hb})

	case "2": // Resend request, for now we just do a SequenceReset
		seqReset, _ := engine.Spec.Sample("4", spec.SampleOptions{OptionalFields: map[uint16]any{123: nil}})
		seqReset.Set(36, fmt.Sprintf("%d", engine.outSeqNum))
		seqReset.Set(123, "Y")
		actions = append(actions, Action{Type: ActionSend, Msg: seqReset})

	case "4": // Sequence Reset
		seqNoTag, _ := msg.FindFrom(36, 0)
		val, err := seqNoTag.AsInt()
		if err != nil {
			actions = append(actions, Action{Type: ActionError, Err: fmt.Errorf("Invalid SeqNo [36] value set")})
		} else if gapFillFlag, _ := msg.Get(123); gapFillFlag != "Y" {
			engine.inSeqNum = val
		}
		return false, actions

	case "5": // Logout
		actions = append(actions, engine.Off()...)

	default: // Passthrough (blocking until session is closed)
		actions = append(actions, Action{Type: ActionDeliver, Msg: *msg})
	}

	return true, actions
}
