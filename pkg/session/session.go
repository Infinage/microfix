package session

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/transport"
)

type Session struct {
	base transport.Connection
	spec      spec.Spec

	senderCompID string
	targetCompID string

	outSeqNum atomic.Int64
	inSeqNum  atomic.Int64

	heartbeatInt int64

	incoming chan message.Message
	errors chan error

	// Stop signal to close channels
	stopFlag chan any
	stopOnce sync.Once
}

func (sess *Session) Incoming() (<-chan message.Message) {
	return sess.incoming
}

func (sess *Session) Errors() (<-chan error) {
	return sess.errors
}

func NewSession(base transport.Connection, specPath string, senderCompID string, 
	targetCompID string, heartbeatInt int64) (*Session, error) {

	sp, err := spec.LoadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to load spec: %v", err)
	}

	if heartbeatInt <= 0 {
		return nil, fmt.Errorf("Heartbeat Interval must be greater than 0")
	}

	// Attempt to sample the messages we will need
	for _, msgType := range []string {"A", "5", "0"} {
		if _, err := sp.Sample(msgType, true, nil); err != nil {
			return nil, fmt.Errorf("Failed to sample message: %v", msgType)
		}
	}
	
	sess := &Session{
		base: base,
		spec: sp,
		senderCompID: senderCompID,
		targetCompID: targetCompID,
		heartbeatInt: heartbeatInt,
		incoming: make(chan message.Message, 1024),
		errors: make(chan error, 10),
		stopFlag: make(chan any),
	}

	// Set the inSeqNum and outSeqNum
	sess.Reset(1, 1)

	// Spawn a goroutine to login and handle messages
	go sess.run()

	return sess, nil
}

// Close the session
func (sess *Session) Close() {
	sess.stopOnce.Do(func() {
		close(sess.stopFlag)
		sess.base.Close()
	})
}

// Send to the connected client, if passthrough is true fields are sent as is
// Otherwise fields such as MsgType, Checksum are calculated fresh and set
func (sess *Session) Send(msg message.Message, passthrough bool) error {
	if !passthrough {
		if err := sess.finalize(&msg); err != nil {
			return err
		}
	}

	select {
	case sess.base.Outgoing() <- msg:
	case <-sess.base.Done():
		return fmt.Errorf("Session closed: %v", msg.String("|"))
	}

	return nil
}

// Reset Sequence numbers
func (s *Session) Reset(in int64, out int64) {
	s.inSeqNum.Store(in)
	s.outSeqNum.Store(out)
}

// Throws an error if missing: [35, 9, 49, 56, 34, 52, 10]
func (s *Session) finalize(msg *message.Message) error {
	if !msg.Contains(35, 9, 49, 56, 34, 52, 10) {
		return fmt.Errorf("Missing required tags: [35, 9, 49, 56, 34, 52, 10]")
	}

	// Set sender / target compId
	msg.Set(49, s.senderCompID)
	msg.Set(56, s.targetCompID)

	// Update OutSeqNum
	msg.Set(34, strconv.Itoa(int(s.outSeqNum.Load())))
	s.outSeqNum.Add(1)

	// Set SendingTime
	msg.Set(52, time.Now().UTC().Format("20060102-15:04:05.000"))

	msgType, _ := msg.Get(35)
	s.spec.Finalize(msg, msgType)

	return nil
}

// Handle Admin type messages: Login, Logout, Heartbeat, TestMessage
// Other input message types are pass into the Incoming channel
func (sess *Session) run() {
	defer sess.Close()

	// Handle logon
	if err := sess.handleLogon(); err != nil {
		sess.errors <- fmt.Errorf("Logon failed: %v", err)
	}

	// Ticker to monitor and auto send heartbeats
	ticker := time.NewTicker(time.Duration(sess.heartbeatInt) * time.Second)
	defer ticker.Stop()

	// Run loop - handle session
	for {
		select {
		case <-ticker.C:
			hb, _ := sess.spec.Sample("0", true, nil)
			sess.Send(hb, false)

		case err := <-sess.base.Errors():
			sess.errors<-err
			return

		case msg, ok := <-sess.base.Incoming():
			if !ok { return }
			sess.handleMessage(&msg)

		case <-sess.stopFlag:
			lo, _ := sess.spec.Sample("5", true, nil)
			sess.Send(lo, false)
			return
		}
	}
}

// Standard checks for MsgType, InSeqNum, Checksum, BodyLength, etc
func (sess *Session) validateMessage(msg *message.Message, eMsgType string) error {
	if msgType, _ := msg.Get(35); msgType != eMsgType {
		return fmt.Errorf("Expected to get a MsgType [35=%v], got %v", eMsgType, msgType)
	}

	// Validate inbound sequencen no
	eSeqNum := sess.inSeqNum.Load()
	if inSeqNum, ok := msg.Get(34); !ok || inSeqNum != fmt.Sprintf("%d", eSeqNum) {
		return fmt.Errorf("Input sequence number mismatch. Expected %v, got %v", eSeqNum, inSeqNum)
	}

	if ok, obs := sess.spec.Validate(msg, spec.Basic); !ok {
		return fmt.Errorf("Message validation failed: %v", obs)
	}

	return nil
}

func (sess *Session) handleLogon() error {
	// Prepare logon message
	logon, _ := sess.spec.Sample("A", false, nil)
	logon.Set(108, fmt.Sprintf("%d", sess.heartbeatInt))
	if err := sess.Send(logon, false); err != nil {
		return err
	}

	// Wait for response with a timeout
	select {
	case msg := <-sess.base.Incoming():
		if err := sess.validateMessage(&msg, "A"); err != nil {
			return err
		}
		return nil

	case <-time.After(5 * time.Second):
		return fmt.Errorf("Logon timeout")
	}
}

func (sess *Session) handleMessage(msg *message.Message) {
		sess.inSeqNum.Add(1)
		msgType, ok := msg.Get(35) 
		// Ensure inSeqNum is correct, validate the message itself with spec

		if !ok {
			sess.errors <-fmt.Errorf("No MsgType found in incoming message: %v", msg.String("|"))
		}

		switch msgType {
		case "1":
			// handle test request

		default:
			sess.incoming<-*msg

		}


}
