package session

import (
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/spec"
)

func TestEngine_LogonFlow(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30, EngineOptions{})

	actions := engine.OnStart(true)

	if len(actions) != 1 || actions[0].Type != ActionSend {
		t.Fatalf("expected logon send action")
	}

	msg := actions[0].Msg
	if mt, _ := msg.Get(35); mt != "A" {
		t.Fatalf("expected MsgType A, got %s", mt)
	}
}

func TestEngine_HandleLogonResponse(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30, EngineOptions{})
	engine.OnStart(true)

	msg, _ := engine.Router.Sample("A", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "1")
	msg.Set(108, "30")
	msg.Finalize()

	actions := engine.OnMessage(&msg, time.Now())

	if engine.state != SessionActive {
		t.Fatalf("expected state Active")
	}
	if len(actions) != 0 {
		t.Fatalf("expected no actions, got %v", actions)
	}
}

func TestEngine_SequenceGap(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30, EngineOptions{})
	engine.state = SessionActive
	engine.inSeqNum = 2

	msg, _ := engine.Router.Sample("D", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "10")
	msg.Finalize()

	actions := engine.OnMessage(&msg, time.Now())

	if len(actions) != 2 || actions[1].Type != ActionSend {
		t.Fatalf("expected resend request")
	}

	mt, _ := actions[1].Msg.Get(35)
	if mt != "2" {
		t.Fatalf("expected MsgType 2, got %s", mt)
	}
}

func TestEngine_HeartbeatOnIdle(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 1, EngineOptions{})
	engine.state = SessionActive
	engine.lastWriteTime = time.Now().Add(-2 * time.Second)

	actions := engine.OnTick(time.Now())

	if len(actions) == 0 || actions[0].Type != ActionSend {
		t.Fatalf("expected heartbeat send")
	}

	mt, _ := actions[0].Msg.Get(35)
	if mt != "0" {
		t.Fatalf("expected heartbeat (0), got %s", mt)
	}
}

func TestEngine_StaleStateAndRecovery(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30, EngineOptions{})
	engine.state = SessionActive
	engine.lastReadTime = time.Now().Add(-31 * time.Second) // Force idle timeout

	// Tick should push us to Stale and send a TestRequest
	actions := engine.OnTick(time.Now())

	if engine.state != SessionStale {
		t.Fatalf("Expected state SessionStale, got %v", engine.state)
	}

	hasTestReq := false
	for _, a := range actions {
		if a.Type == ActionSend {
			if mt, _ := a.Msg.Get(35); mt == "1" {
				hasTestReq = true
			}
		}
	}
	if !hasTestReq {
		t.Fatalf("Expected engine to send TestRequest (1) when going stale")
	}

	// Receiving a Heartbeat with correct TestReqID should recover to Active
	msg, _ := engine.Router.Sample("0", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "2")
	msg.Set(112, engine.testReqID) // Matching ID
	msg.Finalize()

	engine.inSeqNum = 2 // Sync sequence to avoid resend logic
	engine.OnMessage(&msg, time.Now())

	if engine.state != SessionActive {
		t.Fatalf("Expected state to recover to SessionActive, got %v", engine.state)
	}
}

func TestEngine_AcceptGapFill(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30, EngineOptions{})
	engine.state = SessionActive
	engine.inSeqNum = 5 // We expect 5

	// Counterparty sends a Gap Fill skipping us to 10
	msg, _ := engine.Router.Sample("4", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "5")
	msg.Set(123, "Y") // GapFill flag
	msg.Set(36, "10") // NewSeqNo
	msg.Finalize()

	actions := engine.OnMessage(&msg, time.Now())

	// Sequence resets return false for msgAccepted, so inSeqNum isn't auto-incremented by the loop
	// It should be explicitly set to Tag 36 by the handler
	if engine.inSeqNum != 10 {
		t.Fatalf("Expected engine to update InSeqNum to 10, got %d", engine.inSeqNum)
	}

	// Ensure no error actions were generated
	for _, a := range actions {
		if a.Type == ActionError {
			t.Fatalf("Unexpected error action: %v", a.Err)
		}
	}
}

func TestEngine_RejectInvalidLogon(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30, EngineOptions{})
	engine.OnStart(true) // We are LoggingIn, expecting HB=30

	// Server replies with HB=45 (Mismatch)
	msg, _ := engine.Router.Sample("A", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "1")
	msg.Set(108, "45")
	msg.Finalize()

	actions := engine.OnMessage(&msg, time.Now())

	if engine.state != SessionClosed {
		t.Fatalf("Expected state to be SessionClosed due to mismatch, got %v", engine.state)
	}

	// We should see an Error action, a Send action (Logout), and a Close action
	hasLogout := false
	for _, a := range actions {
		if a.Type == ActionSend {
			if mt, _ := a.Msg.Get(35); mt == "5" {
				hasLogout = true
			}
		}
	}
	if !hasLogout {
		t.Fatalf("Expected engine to send Logout (5) on invalid logon")
	}
}
