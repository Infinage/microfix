package session

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/transport"
)

type Session struct {
	// Underlying fix aware socket connection
	// Donot reset this value at anywhere except on NewSession
	base transport.Connection

	// Engine contains and handles the logic
	engine *Engine

	// Communication channel - delivers message post engine processing
	incoming chan message.Message

	// Pub/Sub model - monitor all incoming + outgoing + err + sys events
	logSubs map[chan<- Log]any
	logMu   sync.RWMutex

	// To queue requests from public APIs - Send, ResetSeq, Snapshot, etc
	requests chan userRequest

	// Thread safe flags
	tombstone      atomic.Value // Last snapshot just before run loop ends
	started        atomic.Bool  // start() has been invoked?
	closeRequested atomic.Bool  // Close() method has been called
	closed         atomic.Bool  // run loop has ended

	// Store latest messages by MsgType (Tag 35)
	lastIn  map[string]message.Message
	lastOut map[string]message.Message
}

// Receives appl message post engine processing, DO NOT close the channel
func (sess *Session) Incoming() <-chan message.Message {
	return sess.incoming
}

// Is session's underlying transport channel closed
func (sess *Session) Done() <-chan struct{} {
	if !sess.started.Load() {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	return sess.base.Done()
}

// Starts a SINGLE use session
func NewSession(specPath string, senderCompID string, targetCompID string, heartbeatInt int64, engineOpts EngineOptions) (*Session, error) {
	router, err := spec.NewDefaultRouter(specPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize router: %v", err)
	}

	engine, err := NewEngine(router, senderCompID, targetCompID, heartbeatInt, engineOpts)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		base:     nil,
		engine:   engine,
		incoming: make(chan message.Message, 1024),
		logSubs:  make(map[chan<- Log]any),
		requests: make(chan userRequest, 128),
		lastIn:   make(map[string]message.Message),
		lastOut:  make(map[string]message.Message),
	}

	return sess, nil
}

// Close the session (guard to ensure we dont try to close a non active session)
func (sess *Session) Close() {
	if sess.started.Load() && sess.closeRequested.CompareAndSwap(false, true) {
		select {
		case sess.requests <- closeRequest{}:
		case <-sess.Done():
			// Do nothing, run loop already exiting and defer block
			// will handle tombstone and set sess.closed
		}
	}
}

// Listen for a client connection, call blocks until accepted
// Prevent multiple erraneous starts with atomic checks
func (sess *Session) Listen(addr string) error {
	if sess.started.Load() {
		return fmt.Errorf("Session has already started, please reinitialize a new session")
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
// Prevent multiple erraneous starts with atomic checks
func (sess *Session) Connect(addr string) error {
	if sess.started.Load() {
		return fmt.Errorf("Session has already started, please reinitialize a new session")
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
func (sess *Session) Send(msg message.Message, passthrough bool) error {
	if !sess.started.Load() || sess.closeRequested.Load() {
		return fmt.Errorf("Send failed, session not active: %v", msg.String("|"))
	}

	sess.requests <- messageSendRequest{message: msg, passthrough: passthrough}
	return nil
}

// Query the session status
func (sess *Session) Status() Snapshot {
	// Fresh session
	if !sess.started.Load() {
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
func (sess *Session) ResetSequence(inSeqNum int64, outSeqNum int64) error {
	if !sess.started.Load() || sess.closeRequested.Load() {
		return fmt.Errorf("Session is not active")
	}
	sess.requests <- resetSequenceRequest{inSeqNum: inSeqNum, outSeqNum: outSeqNum}
	return nil
}

// Query the last incoming / outgoing message
func (sess *Session) LastMessage(msgType string, isIncoming bool) *message.Message {
	if msgType == "" || !sess.started.Load() {
		return nil
	}

	// Once run loop has ended, we can safely touch LastIn / LastOut
	if sess.closed.Load() {
		if isIncoming {
			if msg, ok := sess.lastIn[msgType]; ok {
				return &msg
			}
		} else {
			if msg, ok := sess.lastOut[msgType]; ok {
				return &msg
			}
		}
		return nil
	}

	// Create a reply channel for session to respond back on
	reply := make(chan *message.Message)
	sess.requests <- lastMessageRequest{isIncoming: isIncoming, msgType: msgType, reply: reply}

	// Wait until request is fullfilled or done is fired
	select {
	case msg := <-reply:
		return msg
	case <-sess.Done():
		return nil
	}
}

// Returns a channel that receives all Incoming + Outgoing + Error + Sys events
// DO NOT close the channel and remember to unsubscribe after use
func (sess *Session) SubscribeLog() (<-chan Log, func(), error) {
	if sess.closeRequested.Load() {
		return nil, nil, fmt.Errorf("Session is closed")
	}

	// Closure manages the scope
	ch := make(chan Log, 256)
	unsubscribe := func() {
		sess.logMu.Lock()
		defer sess.logMu.Unlock()
		if _, ok := sess.logSubs[ch]; ok {
			delete(sess.logSubs, ch)
			close(ch)
		}
	}

	sess.logMu.Lock()
	sess.logSubs[ch] = nil
	sess.logMu.Unlock()

	return ch, unsubscribe, nil
}

// -------------- INTERNAL FUNCTIONS -------------- //

// Start the session loop as a goroutine, entry point for Unit tests
func (sess *Session) start(conn transport.Connection, isClient bool) {
	sess.base = conn
	sess.started.Store(true)
	go sess.run(isClient)
}

// Non blocking write to all subscribers
func (sess *Session) writeLog(log Log) {
	sess.logMu.RLock()
	defer sess.logMu.RUnlock()
	for ch := range sess.logSubs {
		select {
		case ch <- log:
		default: // Drop if channel if full
		}
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
// Exposed via run loop, so safe to call engine altering methods inside
func (sess *Session) handleSend(msg message.Message, passthrough bool) {
	if !passthrough {
		now := time.Now()
		if err := sess.engine.finalizeMessage(&msg, now); err != nil {
			sess.writeLog(newErrorLog(now, err))
			return
		}
	}

	// Blocking send (or until session / underlying transport is closed)
	select {
	case sess.base.Outgoing() <- msg:
		now := time.Now()
		sess.engine.recordWrite(&msg, now)
		if msgType, ok := msg.Get(35); ok {
			sess.lastOut[msgType] = msg
		}
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

// Users need to subscribed logs
func (sess *Session) closeAllLogs() {
	sess.logMu.Lock()
	defer sess.logMu.Unlock()
	for ch := range sess.logSubs {
		close(ch)
	}
	sess.logSubs = make(map[chan<- Log]any)
}

// Handle Admin type messages: Login, Logout, Heartbeat, TestMessage
// Other input message types are pass into the Incoming channel
func (sess *Session) run(isClient bool) {
	// Cleanup after loop exit
	defer func() {
		sess.writeLog(newSysEventLog(time.Now(), "Session loop Ended"))
		sess.tombstone.Store(sess.engine.Snapshot())
		sess.closeRequested.Store(true)
		sess.closed.Store(true)
		close(sess.incoming)
		sess.closeAllLogs()
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
			if msgType, ok := msg.Get(35); ok {
				sess.lastIn[msgType] = msg
			}
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
