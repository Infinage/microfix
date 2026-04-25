package session

import (
	"fmt"
	"strconv"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/transport"
)

type sessionState int

const (
	SessionDisconnected sessionState = iota
	SessionLoggingIn
	SessionActive
	SessionStale
)

type Session struct {
	// Immutable configs
	Spec         spec.Spec
	base         transport.Connection
	senderCompID string
	targetCompID string
	heartbeatInt int64
	testReqID    string

	// Communication channels
	incoming chan message.Message
	errors   chan error

	// Mutable state
	state         sessionState
	inSeqNum      int64
	outSeqNum     int64
	lastWriteTime time.Time
	lastReadTime  time.Time
}

func (sess *Session) Incoming() <-chan message.Message {
	return sess.incoming
}

func (sess *Session) Errors() <-chan error {
	return sess.errors
}

func (sess *Session) Done() <-chan struct{} {
	return sess.base.Done()
}

func NewSession(base transport.Connection, specPath string, senderCompID string,
	targetCompID string, heartbeatInt int64) (*Session, error) {

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
		if _, err := sp.Sample(msgType, true, nil); err != nil {
			return nil, fmt.Errorf("Failed to sample message: %v", msgType)
		}
	}

	sess := &Session{
		Spec:         sp,
		base:         base,
		senderCompID: senderCompID,
		targetCompID: targetCompID,
		heartbeatInt: heartbeatInt,
		testReqID:    "MICROFIX",

		incoming: make(chan message.Message, 1024),
		errors:   make(chan error, 10),

		inSeqNum:  1,
		outSeqNum: 1,
		state:     SessionDisconnected,
	}

	// Spawn a goroutine to login and handle messages
	go sess.run()

	return sess, nil
}

// Close the session (base.close already behind OnceFlag)
func (sess *Session) Close() {
	sess.base.Close()
}

// Non blocking send to errors channel
func (s *Session) reportError(err error) {
	select {
	case <-s.Done():
		return
	case s.errors <- err:
		// error delivered
	default:
		// Channel is full, drop the error
	}
}

// Send to the connected client, if passthrough is true fields are sent as is
// Otherwise fields such as MsgType, Checksum are calculated fresh and set
func (sess *Session) Send(msg message.Message, passthrough bool) {
	if !passthrough {
		if err := sess.finalize(&msg); err != nil {
			sess.reportError(err)
			return
		}
	}

	// Blocking send (or until session / underlying transport is closed)
	select {
	case sess.base.Outgoing() <- msg:
		sess.lastWriteTime = time.Now()
		sess.outSeqNum++
	case <-sess.Done():
		sess.reportError(fmt.Errorf("Session closed: %v", msg.String("|")))
	}
}

// Returns an error if missing: [35, 9, 49, 56, 34, 52, 10]
func (s *Session) finalize(msg *message.Message) error {
	if !msg.Contains(35, 9, 49, 56, 34, 52, 10) {
		return fmt.Errorf("Missing required tags in OUTBOUND: [35, 9, 49, 56, 34, 52, 10]")
	}

	// Set sender / target compId
	msg.Set(49, s.senderCompID)
	msg.Set(56, s.targetCompID)

	// Update OutSeqNum
	msg.Set(34, strconv.Itoa(int(s.outSeqNum)))

	// Set SendingTime
	msg.Set(52, time.Now().UTC().Format("20060102-15:04:05.000"))

	// Recalculate the bodylen and checksum
	msg.Finalize()

	return nil
}

// Checks for MsgType, InSeqNum, Checksum, BodyLength, TargetCompID, SenderCompID + Spec Validation
func (sess *Session) validate(msg *message.Message) (string, error) {
	msgType, ok := msg.Get(35)
	if !ok {
		return msgType, fmt.Errorf("Missing MsgType tag [35]")
	}

	// Validate that message has InSeqNum, set default value guaranteed to fail
	var received int64 = 0
	if inSeqNumTag, seqNoPos := msg.FindFrom(34, 0); seqNoPos != -1 {
		if val, err := inSeqNumTag.AsInt(); err == nil {
			received = val
		}
	}

	// For values greater than we will trigger resend on `handleAppMessage`
	if received < sess.inSeqNum {
		return msgType, fmt.Errorf("Input sequence number mismatch. Expected %v, got %v", sess.inSeqNum, received)
	}

	// Validate the TargetCompID / SenderCompID: check is swapped
	if senderCompID, _ := msg.Get(49); senderCompID != sess.targetCompID {
		return msgType, fmt.Errorf("SenderCompID [49] mismatch, expected '%v' got '%v'", sess.targetCompID, senderCompID)
	}
	if targetCompID, _ := msg.Get(56); targetCompID != sess.senderCompID {
		return msgType, fmt.Errorf("TargetCompID [56] mismatch, expected '%v' got '%v'", sess.senderCompID, targetCompID)
	}

	// Validate per input spec
	if ok, obs := sess.Spec.Validate(msg, spec.ValidationBasic); !ok {
		return msgType, fmt.Errorf("Message validation failed: %v", obs)
	}

	return msgType, nil
}

// Handle Admin type messages: Login, Logout, Heartbeat, TestMessage
// Other input message types are pass into the Incoming channel
func (sess *Session) run() {
	defer close(sess.errors)
	defer close(sess.incoming)
	defer sess.Close()

	// Ticker to monitor for heartbeats, timeouts
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Start off by sending logon
	logon, _ := sess.Spec.Sample("A", false, nil)
	logon.Set(108, fmt.Sprintf("%d", sess.heartbeatInt))
	sess.Send(logon, false)
	sess.state = SessionLoggingIn

	// Run loop - handle session
	for {
		select {
		case <-sess.Done():
			return

		case err, ok := <-sess.base.Errors():
			if !ok {
				return
			}
			sess.reportError(err)

		case msg, ok := <-sess.base.Incoming():
			if !ok {
				return
			}
			sess.onMessage(&msg)

		case <-ticker.C:
			sess.onTick()
		}
	}
}

// Handle timeouts, track and send heartbeats
func (sess *Session) onTick() {
	hbDuration := time.Second * time.Duration(sess.heartbeatInt)

	// Outgoing idle (send heartbeat)
	if time.Since(sess.lastWriteTime) >= hbDuration {
		hb, _ := sess.Spec.Sample("0", true, nil)
		sess.Send(hb, false)
	}

	// Incoming idle (send test request)
	if since := time.Since(sess.lastReadTime); since >= hbDuration {
		if sess.state != SessionStale {
			tr, _ := sess.Spec.Sample("1", true, nil)
			tr.Set(112, sess.testReqID)
			sess.Send(tr, false)
			sess.state = SessionStale
		} else if since >= hbDuration*3 {
			sess.reportError(fmt.Errorf("Counterparty dead"))
			sess.state = SessionDisconnected
			sess.Close()
		}
	}

	// Timeout logon requests if we did not receive a logon back
	if sess.state == SessionLoggingIn && time.Since(sess.lastWriteTime) > 3*time.Second {
		sess.reportError(fmt.Errorf("Logon timeout"))
		sess.state = SessionDisconnected
		sess.Close()
	}
}

// Logic to respond to messages and set states
func (sess *Session) onMessage(msg *message.Message) {
	sess.lastReadTime = time.Now()

	// Validate the message and return its MsgType
	msgType, verr := sess.validate(msg)
	if verr != nil {
		sess.reportError(verr)
		return
	}

	// Every individual handler returns bool if msg is accepted
	// We will update our inSeqNum only when it is accepted
	var msgAccepted bool

	switch sess.state {
	case SessionDisconnected:
		return

	case SessionLoggingIn:
		if msgType == "A" {
			msgAccepted = sess.handleLogon(msg)
		}

	case SessionActive:
		msgAccepted = sess.handleAppMessage(msg)

	case SessionStale:
		if msgType == "0" {
			reqID, _ := msg.Get(112)
			if reqID != sess.testReqID { // Log warn but continue
				sess.reportError(fmt.Errorf("Expected Heartbeat TestReqID tag [112] to %v", sess.testReqID))
			}
			sess.state = SessionActive
			msgAccepted = true
		}
	}

	// Update inbound sequence number
	if msgAccepted {
		sess.inSeqNum++
	}
}

func (sess *Session) handleLogon(msg *message.Message) bool {
	// Validate heartbeatInt matches
	if hbIntTag, pos := msg.FindFrom(108, 0); pos == -1 {
		sess.reportError(fmt.Errorf("Logon message missing HeartBtInt [108]"))
		return false
	} else if hbInt, err := hbIntTag.AsInt(); err != nil || hbInt != sess.heartbeatInt {
		sess.reportError(fmt.Errorf("Heartbeat Interval in Logon incorrect, expected %v, got %v", sess.heartbeatInt, hbIntTag.Value))
		return false
	}

	// Ensure tag 98 (EncryptMethod) is set
	if _, ok := msg.Get(98); !ok {
		sess.reportError(fmt.Errorf("Missing required tag EncryptMethod [98]"))
		return false
	}

	// Reset the sequence number
	if resetSeqNumFlag, _ := msg.Get(141); resetSeqNumFlag == "Y" {
		sess.inSeqNum, sess.outSeqNum = 1, 1
	}

	// If all good, we proceed to next stage
	sess.state = SessionActive
	return true
}

func (sess *Session) handleAppMessage(msg *message.Message) bool {
	// Get the MsgType
	msgType, _ := msg.Get(35)

	// Trigger a resend request (replay), if inSeqNum greater what we are expecting
	inSeqNumTag, _ := msg.FindFrom(34, 0)
	if inSeqNum, _ := inSeqNumTag.AsInt(); inSeqNum > sess.inSeqNum {
		resend, _ := sess.Spec.Sample("2", true, nil)
		resend.Set(7, fmt.Sprintf("%d", sess.inSeqNum))
		resend.Set(16, fmt.Sprintf("%d", inSeqNum-1))
		sess.Send(resend, false)
		sess.reportError(fmt.Errorf("Expected InSeqNum [34] %v, got %v, triggered resend request.", sess.inSeqNum, inSeqNum))
		return false
	}

	switch msgType {
	case "0": // Already updated InSeqNum, noop

	case "1": // Handle Test Request
		hb, _ := sess.Spec.Sample("0", true, nil)
		if reqId, ok := msg.Get(112); ok && !hb.Set(112, reqId) {
			hb.Insert(1, message.Field{Tag: 112, Value: reqId})
		}
		sess.Send(hb, false) // blocking send

	case "2": // Resend request, for now we just do a SequenceReset
		seqReset, _ := sess.Spec.Sample("4", true, nil)
		seqReset.Set(36, fmt.Sprintf("%d", sess.outSeqNum))
		if !seqReset.Set(123, "Y") {
			seqReset.Insert(len(*msg)-1, message.Field{Tag: 123, Value: "Y"})
		}
		sess.Send(seqReset, false)

	case "4": // Sequence Reset
		seqNoTag, _ := msg.FindFrom(36, 0)
		val, err := seqNoTag.AsInt()
		if err != nil {
			sess.reportError(fmt.Errorf("Invalid SeqNo [36] value set"))
		} else {
			sess.inSeqNum = val
		}
		return false

	case "5": // Logout
		lo, _ := sess.Spec.Sample("5", true, nil)
		sess.Send(lo, false)
		sess.Close()

	default: // Passthrough (blocking until session is closed)
		select {
		case sess.incoming <- *msg:
		case <-sess.Done():
			return false
		}
	}

	return true
}
