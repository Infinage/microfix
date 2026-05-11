package session

import (
	"strings"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

// MockConnection simulates the underlying transport
type MockConnection struct {
	incoming chan message.Message
	outgoing chan message.Message
	errors   chan error
	done     chan struct{}
}

func (m *MockConnection) Incoming() <-chan message.Message { return m.incoming }
func (m *MockConnection) Outgoing() chan<- message.Message { return m.outgoing }
func (m *MockConnection) Errors() <-chan error             { return m.errors }
func (m *MockConnection) Done() <-chan struct{}            { return m.done }
func (m *MockConnection) Close()                           { close(m.done) }

func TestSession_Lifecycle(t *testing.T) {
	mockConn := &MockConnection{
		incoming: make(chan message.Message, 10),
		outgoing: make(chan message.Message, 10),
		errors:   make(chan error, 10),
		done:     make(chan struct{}),
	}

	sess, err := NewSession("FIX44.xml", "SENDER", "TARGET", 30, EngineOptions{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Start the session loop
	sess.start(mockConn, true)

	// Drain out the "Starting session as sys log"
	sysLog := <-sess.Log()
	if sysLog.Type != LogSys {
		t.Errorf("Expected a %s log type, got %v: %v", LogSys, sysLog.Type, sysLog)
	}

	t.Run("SendsLogonOnStart", func(t *testing.T) {
		// Check Outgoing Wire
		select {
		case msg := <-mockConn.outgoing:
			if mt, _ := msg.Get(35); mt != "A" {
				t.Fatalf("expected Logon (A), got %s", mt)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no logon sent to wire")
		}

		// Check Internal Logs
		select {
		case l := <-sess.Log():
			if l.Type != LogSend {
				t.Errorf("expected LogType %v for Logon, got %v: %v", LogSend, l.Type, l)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no logon recorded in logs")
		}
	})

	t.Run("TransitionsToActiveOnLogonResponse", func(t *testing.T) {
		resp, _ := sess.Router().Sample("A", spec.SampleOptions{})
		resp.Set(49, "TARGET")
		resp.Set(56, "SENDER")
		resp.Set(34, "1")
		resp.Set(98, "0")
		resp.Set(108, "30")
		resp.Finalize()

		mockConn.incoming <- resp

		// We expect a LogRecv and NO LogErr
		select {
		case l := <-sess.Log():
			if l.Type == LogErr {
				t.Fatalf("unexpected error during logon: %v", l.Err)
			}
			if l.Type != LogRecv {
				t.Errorf("expected LogRecv, got %v", l.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for logon response log")
		}

		// Poll for state change
		success := false
		deadline := time.Now().Add(250 * time.Millisecond)
		for time.Now().Before(deadline) {
			if sess.Status().State == SessionActive {
				success = true
				break
			}
		}
		if !success {
			t.Errorf("Expected Active state, got %s", sess.Status().State)
		}
	})

	t.Run("SequenceGapTriggersResend", func(t *testing.T) {
		// Send a message with SeqNum 10 (expecting 2)
		msg, _ := sess.Router().Sample("D", spec.SampleOptions{})
		msg.Set(49, "TARGET")
		msg.Set(56, "SENDER")
		msg.Set(34, "10")
		msg.Finalize()

		mockConn.incoming <- msg

		// Should see the RECV log for the gap message
		select {
		case l := <-sess.Log():
			if l.Type != LogRecv {
				t.Errorf("expected Recv log, got %v", l.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no log for incoming gap message")
		}

		// Should see a Err log for InSeqNum mismatch
		select {
		case l := <-sess.Log():
			if l.Type != LogErr || !strings.Contains(l.Err.Error(), "Expected InSeqNum [34]") {
				t.Errorf("expected ERR for InSeqNum, got %v", l.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no log for outgoing ResendRequest")
		}

		// Should see a ResendRequest (35=2) on the wire
		select {
		case out := <-mockConn.outgoing:
			mt, _ := out.Get(35)
			if mt != "2" {
				t.Fatalf("expected ResendRequest (2), got %s", mt)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected resend request on wire")
		}
	})
}

func TestSession_DeliverAppMessage(t *testing.T) {
	mockConn := &MockConnection{
		incoming: make(chan message.Message, 10),
		outgoing: make(chan message.Message, 10),
		errors:   make(chan error, 10),
		done:     make(chan struct{}),
	}

	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	sess.start(mockConn, false) // Start as acceptor

	// Send a logon to drive state to active
	logon, _ := sess.Router().Sample("A", spec.SampleOptions{})
	logon.Set(49, "T")
	logon.Set(56, "S")
	logon.Set(34, "1")
	logon.Set(108, "30")
	logon.Finalize()
	mockConn.incoming <- logon

	// Send an Application Message (New Order Single)
	msg, _ := sess.Router().Sample("D", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "2") // If set to 1 we will get a logout
	msg.Finalize()
	mockConn.incoming <- msg

	// Wait for the message to hit the public Incoming() channel
	select {
	case delivered, ok := <-sess.Incoming():
		if !ok {
			t.Fatalf("Incoming message channel closed")
		}
		if mt, _ := delivered.Get(35); mt != "D" {
			t.Fatalf("Expected delivered message to be New Order Single (D), got %s", mt)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for application message to be delivered")
	}
}

func TestSession_AbruptDisconnect(t *testing.T) {
	mockConn := &MockConnection{
		incoming: make(chan message.Message, 3),
		outgoing: make(chan message.Message, 3),
		errors:   make(chan error, 3),
		done:     make(chan struct{}),
	}

	// Simulate a suddenly dropped TCP connection
	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	sess.start(mockConn, false)
	mockConn.Close()

	// Wait for the session to process the Done signal and clean itself up
	select {
	case <-sess.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Session failed to shut down after underlying connection closed")
	}

	// Verify tombstone state was captured
	status := sess.Status()
	if status.State != SessionClosed {
		t.Errorf("Expected final tombstone state to be SessionClosed, got %v", status.State)
	}
}

func TestSession_DoubleCloseSafety(t *testing.T) {
	mockConn := &MockConnection{
		incoming: make(chan message.Message, 10),
		outgoing: make(chan message.Message, 10),
		errors:   make(chan error, 10),
		done:     make(chan struct{}),
	}

	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	sess.start(mockConn, false)

	// Attempting to close simultaneously or consecutively should not panic or block
	sess.Close()
	sess.Close()

	select {
	case <-sess.Done():
		// Pass. The transport channel gracefully closed and the session tore down without a panic.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Session deadlock on Double Close")
	}
}
