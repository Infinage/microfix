package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/transport"
)

type Session struct {
	// Underlying fix aware socket connection
	base transport.Connection

	// Engine contains and handles the logic
	engine *Engine

	// Communication channels
	incoming chan message.Message
	once     sync.Once

	// Channel to monitor all incoming + outgoing
	logs chan Log
}

func (sess *Session) Incoming() <-chan message.Message {
	return sess.incoming
}

func (sess *Session) Log() <-chan Log {
	return sess.logs
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
		engine:   engine,
		incoming: make(chan message.Message, 1024),
		logs:     make(chan Log, 1024),
	}

	return sess, nil
}

// Close the session (base.close already behind OnceFlag but may need it for mocks)
func (sess *Session) Close() {
	if sess.base != nil {
		sess.once.Do(func() {
			sess.writeLog(newSysEventLog(time.Now(), "Close initiated by user/engine"))
			sess.base.Close()
		})
	}
}

// Listen for a client connection, call blocks until accepted
func (sess *Session) Listen(addr string) error {
	if sess.Status() != SessionNew {
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
func (sess *Session) Connect(addr string) error {
	if sess.Status() != SessionNew {
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

// Send to the connected client, if passthrough is true fields are sent as is
// Otherwise fields such as MsgType, Checksum are calculated fresh and set
func (sess *Session) Send(msg message.Message, passthrough bool) {
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
		sess.engine.RecordWrite(now)
		sess.writeLog(newMessageLog(now, msg, false))
	case <-sess.Done():
		sess.writeLog(newErrorLog(time.Now(), fmt.Errorf("Send failed, session closed: %v", msg.String("|"))))
	}
}

// Returns a copy of the underlying spec object
func (sess *Session) Spec() *spec.Spec {
	return &sess.engine.Spec
}

// Query the session status
func (sess *Session) Status() SessionState {
	return sess.engine.State()
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

	// Timeout and write to non blocking logs channel
	default:
		sess.writeLog(newErrorLog(time.Now(), fmt.Errorf("Message queue full, dropping %v", msg.String("|"))))
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
			sess.writeLog(newErrorLog(time.Now(), action.Err))

		case ActionClose:
			sess.Close()
		}
	}
}

// Handle Admin type messages: Login, Logout, Heartbeat, TestMessage
// Other input message types are pass into the Incoming channel
func (sess *Session) run(isClient bool) {
	defer close(sess.incoming)
	defer sess.writeLog(newSysEventLog(time.Now(), "Session loop Ended"))

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
			return

		// Forward to user's logs
		case err, ok := <-sess.base.Errors():
			if !ok {
				return
			}
			sess.writeLog(newErrorLog(time.Now(), err))

		// Process logic and execute actions as decided by the engine
		case msg, ok := <-sess.base.Incoming():
			if !ok {
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
		}
	}
}
