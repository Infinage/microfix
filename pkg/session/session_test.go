package session

import (
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
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

	// Initialization
	sess, err := NewSession("FIX44.xml", "SENDER", "TARGET", 30)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	t.Run("StartSession", func(t *testing.T) {
		if sess.state != SessionDisconnected {
			t.Errorf("Expected state before start() to be %v got %v", SessionDisconnected, sess.state)
		}

		// Start off the session as a client
		sess.start(mockConn, true)

		// Check that the state is now set to SessionLoggingIn
		time.Sleep(20 * time.Millisecond)
		if sess.state != SessionLoggingIn {
			t.Errorf("Expected state after start() to be %v got %v", SessionLoggingIn, sess.state)
		}
	})

	t.Run("TransitionToActive", func(t *testing.T) {
		// Verify initial Logon was sent
		select {
		case msg := <-mockConn.outgoing:
			if mType, _ := msg.Get(35); mType != "A" {
				t.Fatalf("Expected initial Logon (A), got: %s", mType)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Initial Logon not sent")
		}

		// Feed Logon Response
		resp, _ := sess.Spec.Sample("A", true, nil)
		resp.Set(49, "TARGET")
		resp.Set(56, "SENDER")
		resp.Set(34, "1")
		resp.Set(98, "0")
		resp.Set(108, "30")
		resp.Finalize()
		mockConn.incoming <- resp

		// Wait for internal state update
		time.Sleep(20 * time.Millisecond)
		if sess.state != SessionActive {
			t.Errorf("Session failed to transition to Active, state is: %v", sess.state)
		}
	})

	t.Run("HandleSequenceGap", func(t *testing.T) {
		// We are now Active with inSeqNum = 2 (incremented after Logon)
		// Send message with SeqNum 10 to trigger a gap
		gapMsg, _ := sess.Spec.Sample("D", true, nil)
		gapMsg.Set(49, "TARGET")
		gapMsg.Set(56, "SENDER")
		gapMsg.Set(34, "10")
		gapMsg.Finalize()
		mockConn.incoming <- gapMsg

		// Verify ResendRequest
		select {
		case msg := <-mockConn.outgoing:
			mType, _ := msg.Get(35)
			begin, _ := msg.Get(7)
			end, _ := msg.Get(16)

			if mType != "2" {
				t.Errorf("Expected ResendRequest (2), got: %s", mType)
			}
			// Expected 2 because Logon was 1, so next is 2.
			if begin != "2" || end != "9" {
				t.Errorf("Expected range 2-9, got: %s-%s", begin, end)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("ResendRequest not sent after sequence gap")
		}
	})
}
