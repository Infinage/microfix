package session

import (
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

	sess, err := NewSession("FIX44.xml", "SENDER", "TARGET", 30)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Start the session loop
	sess.start(mockConn, true)

	t.Run("SendsLogonOnStart", func(t *testing.T) {
		// Check Outgoing Wire
		select {
		case msg := <-mockConn.outgoing:
			if mt, _ := msg.Get(35); mt != "A" {
				t.Fatalf("expected Logon (A), got %s", mt)
			}
		case <-time.After(500 * time.Millisecond):
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
		resp, _ := sess.Spec().Sample("A", spec.SampleOptions{})
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

		if sess.Status() != SessionActive {
			t.Errorf("expected Active state, got %s", sess.Status().String())
		}
	})

	t.Run("SequenceGapTriggersResend", func(t *testing.T) {
		// Send a message with SeqNum 10 (expecting 2)
		msg, _ := sess.Spec().Sample("D", spec.SampleOptions{})
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

		// Should see a SEND log for that ResendRequest
		select {
		case l := <-sess.Log():
			if l.Type != LogSend {
				t.Errorf("expected LogSend for ResendRequest, got %v", l.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no log for outgoing ResendRequest")
		}
	})
}
