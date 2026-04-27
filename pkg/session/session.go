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
	base transport.Connection

	// Engine contains and handles the logic
	engine Engine

	// Communication channels
	incoming chan message.Message
	errors   chan error
	once     sync.Once
	closed   atomic.Bool
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

// Starts a SINGLE use session
func NewSession(specPath string, senderCompID string, targetCompID string, heartbeatInt int64) (*Session, error) {
	engine, err := NewEngine(specPath, senderCompID, targetCompID, heartbeatInt)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		base:     nil,
		engine:   *engine,
		incoming: make(chan message.Message, 1024),
		errors:   make(chan error, 10),
	}

	return sess, nil
}

// Close the session (base.close already behind OnceFlag but may need it for mocks)
func (sess *Session) Close() {
	if sess.base != nil {
		sess.once.Do(func() {
			sess.closed.Store(true)
			sess.base.Close()
		})
	}
}

// Listen for a client connection, call blocks until accepted
func (sess *Session) Listen(addr string) error {
	if sess.closed.Load() {
		return fmt.Errorf("Session has been closed, please use a new session")
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
func (sess *Session) Connect(addr string) error {
	if sess.closed.Load() {
		return fmt.Errorf("Session has been closed, please use a new session")
	}

	conn, err := transport.Dial(addr)
	if err != nil {
		return err
	}

	// Start session as a client
	sess.start(conn, true)
	return nil
}

// Send to the connected client, if passthrough is true fields are sent as is
// Otherwise fields such as MsgType, Checksum are calculated fresh and set
func (sess *Session) Send(msg message.Message, passthrough bool) {
	if !passthrough {
		if err := sess.engine.FinalizeMessage(&msg, time.Now()); err != nil {
			sess.reportError(err)
			return
		}
	}

	// Blocking send (or until session / underlying transport is closed)
	select {
	case sess.base.Outgoing() <- msg:
		sess.engine.RecordWrite(time.Now())
	case <-sess.Done():
		sess.reportError(fmt.Errorf("Session closed: %v", msg.String("|")))
	}
}

// Returns a copy of the underlying spec object
func (sess *Session) Spec() spec.Spec {
	return sess.engine.Spec
}

// -------------- INTERNAL FUNCTIONS -------------- //

// Start the session loop as a goroutine, entry point for Unit tests
func (sess *Session) start(conn transport.Connection, isClient bool) {
	sess.base = conn
	go sess.run(isClient)
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

// Blocking delivery of message to user's incoming channel
func (sess *Session) deliverMessage(msg *message.Message) {
	select {
	// Can't send since underlying transport is closed
	case <-sess.Done():

	// Send the message to users incoming channel
	case sess.incoming <- *msg:
	}
}

// Execute actions from Fix Engine
func (sess *Session) execute(actions []Action) {
	for _, action := range actions {
		switch action.Type {
		case ActionSend:
			sess.Send(action.Msg, false)

		case ActionDeliver:
			sess.deliverMessage(&action.Msg)

		case ActionError:
			sess.reportError(action.Err)

		case ActionClose:
			sess.Close()
		}
	}
}

// Handle Admin type messages: Login, Logout, Heartbeat, TestMessage
// Other input message types are pass into the Incoming channel
func (sess *Session) run(isClient bool) {
	defer close(sess.errors)
	defer close(sess.incoming)

	// Ticker to monitor for heartbeats, timeouts
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Turn on Fix engine as a Server / Client
	actions := sess.engine.OnStart(isClient)
	sess.execute(actions)

	// Run loop - handle session
	for {
		select {
		case <-sess.Done():
			return

		// Forward to user's error channel
		case err, ok := <-sess.base.Errors():
			if !ok {
				return
			}
			sess.reportError(err)

		// Process logic and execute actions as decided by the engine
		case msg, ok := <-sess.base.Incoming():
			if !ok {
				return
			}
			actions := sess.engine.OnMessage(&msg, time.Now())
			sess.execute(actions)

		// Notify engine of a single tick to run its timed logic
		case <-ticker.C:
			actions := sess.engine.OnTick(time.Now())
			sess.execute(actions)
		}
	}
}
