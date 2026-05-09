package session

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/transport"
)

type Session struct {
	// Underlying fix aware socket connection
	// We use base == nil to detect if session is fresh
	// Donot reset this value at anywhere except on NewSession
	base transport.Connection

	// Engine contains and handles the logic
	engine *Engine

	// Communication channels
	incoming chan message.Message

	// Channel to monitor all incoming + outgoing
	logs chan Log

	// To queue requests from public APIs - Send, ResetSeq, Snapshot, etc
	requests chan userRequest

	// Track if the session has been closed, also store the last snapshot "tomstone"
	tombstone atomic.Value
	closed    atomic.Bool
}

func (sess *Session) Incoming() <-chan message.Message {
	return sess.incoming
}

func (sess *Session) Log() <-chan Log {
	return sess.logs
}

// Is session's underlying transport channel closed
func (sess *Session) Done() <-chan struct{} {
	if sess.base == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	return sess.base.Done()
}

// Starts a SINGLE use session
func NewSession(specPath string, senderCompID string, targetCompID string, heartbeatInt int64, engineOpts EngineOptions) (*Session, error) {
	engine, err := NewEngine(specPath, senderCompID, targetCompID, heartbeatInt, engineOpts)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		base:     nil,
		engine:   engine,
		incoming: make(chan message.Message, 1024),
		logs:     make(chan Log, 1024),
		requests: make(chan userRequest, 128),
	}

	return sess, nil
}

// Close the session (gaurd to ensure we dont try to close a non active session)
func (sess *Session) Close() {
	if sess.base != nil && !sess.closed.Load() {
		select {
		case sess.requests <- closeRequest{}:
		case <-sess.Done():
			sess.closed.Store(true)
		}
	}
}

// Listen for a client connection, call blocks until accepted
// On session being closed, base will still be not nil and we err out
func (sess *Session) Listen(addr string) error {
	if sess.base != nil {
		return fmt.Errorf("Session has already started and cannot be reused")
	}

	conn, err := transport.Listen1(addr)
	if err != nil {
		return err
	}

	// Start session as a server
	sess.start(conn, false)
	return nil
}

// Connect to a server, call blocks until connected
// On session being closed, base will still be not nil and we err out
func (sess *Session) Connect(addr string) error {
	if sess.base != nil {
		return fmt.Errorf("Session has already started and cannot be reused")
	}

	conn, err := transport.Dial(addr)
	if err != nil {
		return err
	}

	// Start session as a client
	sess.start(conn, true)
	return nil
}

// Returns the underlying Router object
func (sess *Session) Router() *spec.Router {
	return &sess.engine.Router
}

// Send to the connected client, if passthrough is true fields are sent as is
// Otherwise fields such as MsgType, Checksum are calculated fresh and set
func (sess *Session) Send(msg message.Message, passthrough bool) {
	if sess.base == nil {
		sess.writeLog(newErrorLog(time.Now(), fmt.Errorf("Send failed, session not started: %v", msg.String("|"))))
		return
	}

	if !sess.closed.Load() {
		sess.requests <- messageSendRequest{message: msg, passthrough: passthrough}
	}
}

// Query the session status
func (sess *Session) Status() Snapshot {
	// Fresh session
	if sess.base == nil {
		return Snapshot{State: SessionNew}
	}

	// Closed session, return latest snapshot, set on run loop exit
	if sess.closed.Load() {
		return sess.tombstone.Load().(Snapshot)
	}

	// Session active, request to run loop
	reply := make(chan Snapshot, 1)
	sess.requests <- snapshotRequest{reply: reply}

	// Wait for a response from run loop
	// If sess is closed while waiting, try to retrieve tombstone
	// On failure, return a dummy with state set
	select {
	case snap := <-reply:
		return snap
	case <-sess.Done():
		snap, ok := sess.tombstone.Load().(Snapshot)
		if !ok {
			snap = Snapshot{State: SessionClosed}
		}
		return snap
	}
}

// Reset the session number (queue to run loop)
func (sess *Session) ResetSequence(inSeqNum int64, outSeqNum int64) {
	if sess.base == nil || sess.closed.Load() {
		return
	}
	sess.requests <- resetSequence{inSeqNum: inSeqNum, outSeqNum: outSeqNum}
}

// -------------- INTERNAL FUNCTIONS -------------- //

// Start the session loop as a goroutine, entry point for Unit tests
func (sess *Session) start(conn transport.Connection, isClient bool) {
	sess.base = conn
	go sess.run(isClient)
}

// Non blocking write to logs channel
func (sess *Session) writeLog(log Log) {
	select {
	case sess.logs <- log:
	default: // Drop if channel if full
	}
}

// Blocking delivery of message to user's incoming channel
func (sess *Session) deliverMessage(msg *message.Message) {
	select {
	// Send the message to users incoming channel
	case sess.incoming <- *msg:

	// On client queue being full, write to non blocking logs channel
	default:
		sess.writeLog(newErrorLog(time.Now(), fmt.Errorf("Message queue full, dropping %v", msg.String("|"))))
	}
}

// From World / Run loop to the external client we are connected to
func (sess *Session) handleSend(msg message.Message, passthrough bool) {
	if !passthrough {
		now := time.Now()
		if err := sess.engine.FinalizeMessage(&msg, now); err != nil {
			sess.writeLog(newErrorLog(now, err))
			return
		}
	}

	// Blocking send (or until session / underlying transport is closed)
	select {
	case sess.base.Outgoing() <- msg:
		now := time.Now()
		sess.engine.RecordWrite(&msg, now)
		sess.writeLog(newMessageLog(now, msg, false))
	case <-sess.Done():
		sess.writeLog(newErrorLog(time.Now(), fmt.Errorf("Send failed, session closed: %v", msg.String("|"))))
	}
}

// Execute actions from Fix Engine
func (sess *Session) execute(actions []Action) {
	for _, action := range actions {
		switch action.Type {
		case ActionSend:
			sess.handleSend(action.Msg, false)

		case ActionDeliver:
			sess.deliverMessage(&action.Msg)

		case ActionError:
			sess.writeLog(newErrorLog(time.Now(), action.Err))

		case ActionLog:
			sess.writeLog(newSysEventLog(time.Now(), action.Event))

		case ActionClose:
			sess.Close()
		}
	}
}

// Handle Admin type messages: Login, Logout, Heartbeat, TestMessage
// Other input message types are pass into the Incoming channel
func (sess *Session) run(isClient bool) {
	// Cleanup after loop exit
	defer func() {
		sess.writeLog(newSysEventLog(time.Now(), "Session loop Ended"))
		sess.tombstone.Store(sess.engine.Snapshot())
		sess.closed.Store(true)
		close(sess.incoming)
		close(sess.logs)
	}()

	// Ticker to monitor for heartbeats, timeouts
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	role := "Acceptor"
	if isClient {
		role = "Initiator"
	}
	sess.writeLog(newSysEventLog(time.Now(), fmt.Sprintf("Starting session as %s", role)))

	// Turn on Fix engine as a Server / Client
	actions := sess.engine.OnStart(isClient)
	sess.execute(actions)

	// Run loop - handle session
	for {
		select {
		case <-sess.Done():
			sess.engine.OnDisconnect()
			return

		// Forward to user's logs
		case err, ok := <-sess.base.Errors():
			if !ok {
				sess.engine.OnDisconnect()
				return
			}
			sess.writeLog(newErrorLog(time.Now(), err))

		// Process logic and execute actions as decided by the engine
		case msg, ok := <-sess.base.Incoming():
			if !ok {
				sess.engine.OnDisconnect()
				return
			}
			now := time.Now()
			sess.writeLog(newMessageLog(now, msg, true))
			actions := sess.engine.OnMessage(&msg, now)
			sess.execute(actions)

		// Notify engine of a single tick to run its timed logic
		case <-ticker.C:
			actions := sess.engine.OnTick(time.Now())
			sess.execute(actions)

		// Handle requests from World
		case req, ok := <-sess.requests:
			if !ok {
				sess.engine.OnDisconnect()
				return
			}
			req.apply(sess)
		}

	}
}
