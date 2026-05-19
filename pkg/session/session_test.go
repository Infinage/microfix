package session

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/transport"
)

func TestSession_Lifecycle(t *testing.T) {
	mockConn := transport.NewMockConnection(10)

	sess, err := NewSession("FIX44.xml", "SENDER", "TARGET", 30, EngineOptions{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Start the session loop
	sess.Start(mockConn, true)

	// Subscribe to logs channel
	logCh, unsubscribe, err := sess.SubscribeLog()
	if err != nil {
		t.Fatal("Log subscription failed")
	}
	defer unsubscribe()

	// Drain out the "Starting session as sys log"
	if sysLog := <-logCh; sysLog.Type != LogSys {
		t.Errorf("Expected a %s log type, got %v: %v", LogSys, sysLog.Type, sysLog)
	}

	t.Run("SendsLogonOnStart", func(t *testing.T) {
		// Check Outgoing Wire
		select {
		case msg := <-mockConn.OutgoingCh:
			if mt, _ := msg.Get(35); mt != "A" {
				t.Fatalf("expected Logon (A), got %s", mt)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no logon sent to wire")
		}

		// Check Internal Logs
		select {
		case l := <-logCh:
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

		mockConn.IncomingCh <- resp

		// We expect a LogRecv and NO LogErr
		select {
		case l := <-logCh:
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

		mockConn.IncomingCh <- msg

		// Should see the RECV log for the gap message
		select {
		case l := <-logCh:
			if l.Type != LogRecv {
				t.Errorf("expected Recv log, got %v", l.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no log for incoming gap message")
		}

		// Should see a Err log for InSeqNum mismatch
		select {
		case l := <-logCh:
			if l.Type != LogErr || !strings.Contains(l.Err.Error(), "Expected InSeqNum [34]") {
				t.Errorf("expected ERR for InSeqNum, got %v", l.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("no log for outgoing ResendRequest")
		}

		// Should see a ResendRequest (35=2) on the wire
		select {
		case out := <-mockConn.OutgoingCh:
			mt, _ := out.Get(35)
			if mt != "2" {
				t.Fatalf("expected ResendRequest (2), got %s", mt)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected resend request on wire")
		}
	})

	t.Run("Verify LastIn and LastOut messages recorded", func(t *testing.T) {
		expect := []struct {
			msgType     string
			isIncoming  bool
			shouldExist bool
		}{
			{"A", false, true},  // Outgoing Logon (from start)
			{"A", true, true},   // Incoming Logon (from transition active)
			{"D", true, true},   // Incoming NewOrderSingle (the gap message)
			{"2", false, true},  // Outgoing ResendRequest (triggered by gap)
			{"D", false, false}, // Never sent an outgoing NewOrderSingle
			{"0", true, false},  // Never received an incoming Heartbeat
		}

		for _, scenario := range expect {
			mapName := "LastOut"
			if scenario.isIncoming {
				mapName = "LastIn"
			}

			msg := sess.LastMessage(scenario.msgType, scenario.isIncoming)

			if scenario.shouldExist && msg == nil {
				t.Errorf("expected to find message of type '%v' in '%v'", scenario.msgType, mapName)
			} else if !scenario.shouldExist && msg != nil {
				t.Errorf("did not expect to find message of type '%v' in '%v', but found one", scenario.msgType, mapName)
			} else if scenario.shouldExist && msg != nil {
				if mt, _ := msg.Get(35); mt != scenario.msgType {
					t.Errorf("expected retrieved msg type to be '%v', got: '%v'", scenario.msgType, mt)
				}
			}
		}
	})
}

func TestSession_DeliverAppMessage(t *testing.T) {
	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	mockConn := transport.NewMockConnection(10)
	sess.Start(mockConn, false) // Start as acceptor

	// Send a logon to drive state to active
	logon, _ := sess.Router().Sample("A", spec.SampleOptions{})
	logon.Set(49, "T")
	logon.Set(56, "S")
	logon.Set(34, "1")
	logon.Set(108, "30")
	logon.Finalize()
	mockConn.IncomingCh <- logon

	// Send an Application Message (New Order Single)
	msg, _ := sess.Router().Sample("D", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "2") // If set to 1 we will get a logout
	msg.Finalize()
	mockConn.IncomingCh <- msg

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

	// Verify LastIn tracked the app message successfully
	lastD := sess.LastMessage("D", true)
	if lastD == nil {
		t.Errorf("Expected LastMessage('D', true) to be recorded, got nil")
	} else if mt, _ := lastD.Get(35); mt != "D" {
		t.Errorf("Expected LastMessage type D, got %v", mt)
	}
}

func TestSession_AbruptDisconnect(t *testing.T) {
	// Simulate a suddenly dropped TCP connection
	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	mockConn := transport.NewMockConnection(3)
	sess.Start(mockConn, false)
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
	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	mockConn := transport.NewMockConnection(10)
	sess.Start(mockConn, false)

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

func TestSession_LogConcurrency(t *testing.T) {
	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	mockConn := transport.NewMockConnection(100)
	sess.Start(mockConn, true)

	var wg sync.WaitGroup
	workers := 50
	iterations := 100

	// Start a goroutine to drain out any outgoing messages
	go mockConn.Drain()

	// Spammer Goroutines: Constantly Subscribe and Unsubscribe
	for range workers {
		wg.Go(func() {
			for range iterations {
				ch, unsub, err := sess.SubscribeLog()
				if err == nil {
					// Drain any immediate logs to prevent blocking if buffer fills
					select {
					case <-ch:
					default:
					}
					unsub()
				}
				// Micro-sleep to force context switching and interleaved operations
				time.Sleep(time.Microsecond)
			}
		})
	}

	// Writer Goroutines: Constantly trigger internal writeLog calls
	for range workers {
		wg.Go(func() {
			for range iterations {
				// Sending regular messages will trigger the run loop to call writeLog
				msg, _ := sess.Router().Sample("0", spec.SampleOptions{})
				_ = sess.Send(msg, false)
				time.Sleep(time.Microsecond)
			}
		})
	}

	// Wait for all concurrent readers and writers to finish
	wg.Wait()

	// Shut down the session while it's still potentially processing queued sends
	sess.Close()

	select {
	case <-sess.Done():
		// Success! If we reach here, it means no concurrent map read/write panics
	case <-time.After(2 * time.Second):
		t.Fatal("Session deadlock during concurrent shutdown")
	}
}

func TestSession_PreStartLogging(t *testing.T) {
	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})

	// Subscribe BEFORE the session connects/starts
	// Proves that log registration isn't dependent on the run() loop.
	logCh, unsub, err := sess.SubscribeLog()
	if err != nil {
		t.Fatal("Failed to subscribe to logs pre-start")
	}
	defer unsub()

	// Create a mock connection and start the session
	mockConn := transport.NewMockConnection(10)
	sess.Start(mockConn, false)

	// The very first thing start()->run() does is emit a SysEvent log.
	// Since we subscribed before starting, we should catch this log perfectly.
	select {
	case l := <-logCh:
		if l.Type != LogSys || !strings.Contains(fmt.Sprint(l.Text), "Starting session") {
			t.Errorf("Expected startup log, got: %v", l)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for pre-subscribed startup log")
	}
}

func TestSession_SlowConsumerDoesNotBlock(t *testing.T) {
	sess, _ := NewSession("FIX44.xml", "S", "T", 30, EngineOptions{})
	mockConn := transport.NewMockConnection(100)
	sess.Start(mockConn, true)

	// Subscribe but purposefully NEVER read from this channel
	_, unsub, err := sess.SubscribeLog()
	if err != nil {
		t.Fatal("Failed to subscribe logs")
	}
	defer unsub()

	// Start a goroutine to drain out any outgoing messages
	go mockConn.Drain()

	// Try to overwhelm the session with log-generating actions
	msg, _ := sess.Router().Sample("0", spec.SampleOptions{})
	for range 300 {
		_ = sess.Send(msg, false)
	}

	// If the session handles the unread channel properly via the `default:` drop case,
	// the Status() request will succeed immediately. If it blocks, it means the `run()` loop is frozen.
	status := sess.Status()
	if status.State == SessionClosed {
		t.Fatal("Session crashed due to slow consumer")
	}

	sess.Close()
}
