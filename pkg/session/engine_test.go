package session

import (
	"strings"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

// setupEngine creates a test Engine using:
//   - SenderCompID = "S"
//   - TargetCompID = "T"
//   - HeartBeatInt = 30
//
// If applPaths is empty, a default router is created from specPath.
// Otherwise a FIXT-style router is created using specPath as the
// session spec and applPaths as application-level specs.
func setupEngine(t *testing.T, specPath string, applPaths []string) *Engine {
	t.Helper()

	var err error
	var router *spec.Router
	var engine *Engine

	if len(applPaths) == 0 {
		if router, err = spec.NewDefaultRouter(specPath); err != nil {
			t.Fatalf("Failed to initialize default router: %v", err)
		}
	} else {
		if router, err = spec.NewRouter(specPath, applPaths); err != nil {
			t.Fatalf("Failed to initialize router: %v", err)
		}
	}

	if engine, err = NewEngine(router, "S", "T", 30, EngineOptions{}); err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	return engine
}

func buildMessage(t *testing.T, engine *Engine, msgType string, extraTags map[uint16]string, sampleOpts spec.SampleOptions) message.Message {
	t.Helper()

	msg, err := engine.Router.Sample(msgType, sampleOpts)
	if err != nil {
		t.Fatalf("Failed to build message: %v", err)
	}

	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "1")

	for k, v := range extraTags {
		msg.Set(k, v)
	}

	msg.Finalize()
	return msg
}

func TestEngine_LogonFlow(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	actions := engine.OnStart(true)

	if len(actions) != 1 || actions[0].Type != ActionSend {
		t.Fatalf("expected logon send action")
	}

	msg := actions[0].Msg
	if mt, _ := msg.Get(35); mt != "A" {
		t.Fatalf("expected MsgType A, got %s", mt)
	}
}

func TestEngine_HandleLogonRequest(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.OnStart(false)

	// Building a heartbeat while engine is looking for logon
	msg := buildMessage(t, engine, "0", nil, spec.SampleOptions{})

	// We expect a reject request
	actions := engine.OnMessage(&msg, time.Now())
	if engine.state != SessionListening {
		t.Fatal("Expected state Listening")
	} else if len(actions) != 1 || actions[0].Type != ActionSend {
		t.Fatal("Expected a single Action of type Send")
	} else if msg, _ := actions[0].Msg.Get(58); msg != "First message not a logon" {
		t.Fatalf("Expected Reject request with 'First message not a logon', got %v", msg)
	}

	// Building a logon
	msg = buildMessage(t, engine, "A", nil, spec.SampleOptions{})

	// Engine should send a logon response
	actions = engine.OnMessage(&msg, time.Now())
	if engine.state != SessionActive {
		t.Errorf("Expected state Active, got %v", engine.state)
	} else if len(actions) != 2 || actions[0].Type != ActionLog || actions[1].Type != ActionSend {
		t.Error("Expected engine to log state transition and send logon response back")
	} else if msgType, _ := actions[1].Msg.Get(35); msgType != "A" {
		t.Errorf("Expected MsgType logon, got %v", msgType)
	}
}

func TestEngine_HandleLogonResponse(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.OnStart(true)

	msg := buildMessage(t, engine, "A", map[uint16]string{108: "30"}, spec.SampleOptions{})
	actions := engine.OnMessage(&msg, time.Now())

	if engine.state != SessionActive {
		t.Fatalf("expected state Active")
	}
	if len(actions) != 0 {
		t.Fatalf("expected no actions, got %v", actions)
	}
}

func TestEngine_SequenceGap(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.state = SessionActive
	engine.inSeqNum = 2

	// Expecting inSeqNum 2 but we set SeqNum as 10
	msg := buildMessage(t, engine, "D", map[uint16]string{34: "10"}, spec.SampleOptions{})
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
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.heartbeatInt = 1
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
	engine := setupEngine(t, "FIX44.xml", nil)
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
	msg := buildMessage(t, engine, "0", map[uint16]string{112: engine.testReqID}, spec.SampleOptions{})
	engine.OnMessage(&msg, time.Now())

	if engine.state != SessionActive {
		t.Fatalf("Expected state to recover to SessionActive, got %v", engine.state)
	}
}

func TestEngine_PossDupBypass(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.state = SessionActive
	engine.inSeqNum = 5 // Engine is expecting 5

	// Sample message with stale SeqNum and ensuring PossDup optional field is sampled
	msg := buildMessage(t, engine, "D", map[uint16]string{34: "3"},
		spec.SampleOptions{OptionalFields: map[uint16]any{43: nil}})

	t.Run("PossDup=N rejects sequence mismatch", func(t *testing.T) {
		msg.Set(43, "N")
		msg.Finalize()

		errEncountered := false
		for _, a := range engine.OnMessage(&msg, time.Now()) {
			if a.Type == ActionError && strings.Contains(a.Err.Error(), "sequence number mismatch") {
				errEncountered = true
				break
			}
		}

		if !errEncountered {
			t.Error("Engine incorrectly accepted a message with a sequence mismatch")
		}
	})

	t.Run("PossDup=Y bypasses sequence mismatch", func(t *testing.T) {
		msg.Set(43, "Y")
		msg.Finalize()

		for _, a := range engine.OnMessage(&msg, time.Now()) {
			if a.Type == ActionError && strings.Contains(a.Err.Error(), "sequence number mismatch") {
				t.Fatalf("Engine incorrectly rejected a PossDup=Y message: %v", a.Err)
			}
		}
	})
}

func TestEngine_AcceptGapFill(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.state = SessionActive
	engine.inSeqNum = 5 // We expect 5

	// Counterparty sends a Gap Fill skipping us to 10 (GapFill [123], NewSeqNo [36])
	msg := buildMessage(t, engine, "4", map[uint16]string{34: "5", 123: "Y", 36: "10"}, spec.SampleOptions{})
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
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.OnStart(true) // We are LoggingIn, expecting HB=30

	// Server replies with HB=45 (Mismatch)
	msg := buildMessage(t, engine, "A", map[uint16]string{108: "45"}, spec.SampleOptions{})
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

func TestEngine_InboundValidation(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	t.Run("Sending time accuracy problem", func(t *testing.T) {
		msg, err := message.MessageFromString("8=FIX.4.4|9=42|35=0|49=T|56=S|34=4|52=20260404-12:00:00Z|10=253|", "|")
		if err != nil {
			t.Fatalf("Failed to parse test string: %v", err)
		}

		err = engine.validate(&msg, time.Now())
		if err == nil || !strings.Contains(err.Error(), "SendingTime accuracy problem [52]") {
			t.Errorf("Expected SendingTime accuracy error, got: '%v'", err)
		}
	})
}

func TestEngine_OutboundValidation(t *testing.T) {
	// Expect some slow down since this will construct a default
	// router with all FIX40 - FIX50SP02 XML specs
	engine := setupEngine(t, "FIXT11.xml", []string{"FIX44.xml"})
	engine.senderCompID = "CLIENT"
	engine.targetCompID = "SERVER"

	t.Run("Missing core required tags", func(t *testing.T) {
		// Missing Tag 35 (MsgType)
		raw := "8=FIXT.1.1|9=00|49=CLIENT|56=SERVER|34=1|52=20260510-02:28:07.725|10=000|"
		msg, err := message.MessageFromString(raw, "|")
		if err != nil {
			t.Fatalf("Failed to parse test string: %v", err)
		}

		err = engine.finalizeMessage(&msg, time.Now())
		if err == nil {
			t.Error("Expected FinalizeMessage to fail due to missing core tags, but it succeeded")
		} else if !strings.Contains(err.Error(), "OUTBOUND missing required session fields") {
			t.Errorf("Expected core tag error, got: %v", err)
		}
	})

	t.Run("Missing message specific required tags", func(t *testing.T) {
		// 35=1 (TestRequest) explicitly requires Tag 112 (TestReqID)
		// It has all 8 core tags, so it passes the initial safety check, but fails the spec validation.
		raw := "8=FIXT.1.1|9=00|35=1|49=CLIENT|56=SERVER|34=35|52=20260510-02:28:07.725|10=000|"
		msg, err := message.MessageFromString(raw, "|")
		if err != nil {
			t.Fatalf("Failed to parse test string: %v", err)
		}

		err = engine.finalizeMessage(&msg, time.Now())
		if err == nil {
			t.Error("Expected FinalizeMessage to fail due to missing Tag 112, but it succeeded")
		} else if !strings.Contains(err.Error(), "Missing required field tag [112]") {
			t.Errorf("Expected Tag 112 error, got: %v", err)
		}
	})

	t.Run("Unsupported / Invalid ApplVerID", func(t *testing.T) {
		// Sample heartbeat with invalid ApplVerID [1128]
		raw := "8=FIXT.1.1|9=65|35=0|49=STRING|56=STRING|34=704|52=20260510-09:11:48.977|1128=99|10=199|"
		msg, err := message.MessageFromString(raw, "|")
		if err != nil {
			t.Fatalf("Failed to parse test string: %v", err)
		}

		ok, obs := engine.validateAgainstSpec(&msg, spec.ValidationBasic)
		if ok {
			t.Error("Expected validateAgainstSpec to fail due to invalid ApplVerID, but passed")
		} else if len(obs) != 1 || !strings.HasPrefix(obs[0], "Message specifies unsupported ApplVerID [1128]") {
			t.Errorf("Expected ApplVerID invalid err, got: %v", obs)
		}
	})
}

func TestEngine_StrictInboundValidation(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)

	t.Run("Reject malformed SendingTime format", func(t *testing.T) {
		msg, _ := message.MessageFromString("8=FIX.4.4|9=00|35=0|49=T|56=S|34=1|52=INVALID|10=000|", "|")
		err := engine.validate(&msg, time.Now())
		if err == nil || !strings.Contains(err.Error(), "Invalid SendingTime [52]") {
			t.Fatalf("expected invalid sending time error, got %v", err)
		}
	})

	t.Run("Reject SenderCompID mismatch with Hard Disconnect", func(t *testing.T) {
		// Msg sent by "WRONG", but we expect "T"
		msg, _ := message.MessageFromString("8=FIX.4.4|9=00|35=0|49=WRONG|56=S|34=1|52=20260510-12:00:00.000|10=000|", "|")
		err := engine.validate(&msg, time.Now())
		if err == nil || !strings.Contains(err.Error(), "SenderCompID [49] mismatch") {
			t.Fatalf("expected SenderCompID mismatch error, got %v", err)
		}

		// Verify OnMessage converts a non-RejectError (like CompID mismatch) into a Logout + Close
		actions := engine.OnMessage(&msg, time.Now())
		if len(actions) != 3 || actions[2].Type != ActionClose {
			t.Fatalf("Expected hard disconnect on CompID mismatch, got %v", actions)
		}
	})

	t.Run("Reject BeginString mismatch", func(t *testing.T) {
		// Engine is FIX.4.4, but msg is FIX.4.2
		msg, _ := message.MessageFromString("8=FIX.4.2|9=00|35=0|49=T|56=S|34=1|52=20260510-12:00:00.000|10=000|", "|")
		err := engine.validate(&msg, time.Now())
		if err == nil || !strings.Contains(err.Error(), "BeginString mistmatch") {
			t.Fatalf("expected BeginString mismatch error, got %v", err)
		}
	})
}

func TestEngine_OutboundSequenceIncrementRules(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.outSeqNum = 10

	t.Run("Normal messages increment OutSeqNum", func(t *testing.T) {
		msg, _ := engine.Router.Sample("0", spec.SampleOptions{})
		engine.recordWrite(&msg, time.Now())
		if engine.outSeqNum != 11 {
			t.Errorf("Expected outSeqNum 11, got %v", engine.outSeqNum)
		}
	})

	t.Run("Sequence Reset (GapFill) does NOT increment", func(t *testing.T) {
		msg, _ := engine.Router.Sample("4", spec.SampleOptions{OptionalFields: map[uint16]any{123: nil}})
		msg.Set(123, "Y")
		engine.recordWrite(&msg, time.Now())
		if engine.outSeqNum != 11 { // Remains 11!
			t.Errorf("Expected outSeqNum to remain 11, got %v", engine.outSeqNum)
		}
	})
}

func TestEngine_LogoutFlow(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.state = SessionActive

	// Logout message
	msg := buildMessage(t, engine, "5", nil, spec.SampleOptions{})
	actions := engine.OnMessage(&msg, time.Now())

	if engine.state != SessionClosed {
		t.Errorf("Expected engine state SessionClosed, got %v", engine.state)
	}

	if len(actions) != 2 || actions[0].Type != ActionSend || actions[1].Type != ActionClose {
		t.Fatalf("Expected [ActionSend(Logout), ActionClose], got %v", actions)
	}
}

func TestEngine_ResendRequestBuildsCorrectTags(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.state = SessionActive
	engine.inSeqNum = 100 // We expect 100

	// Counterparty sends SeqNum: 105
	msg := buildMessage(t, engine, "D", map[uint16]string{34: "105"}, spec.SampleOptions{})
	actions := engine.OnMessage(&msg, time.Now())

	if len(actions) < 2 {
		t.Fatalf("Expected actions, got %v", actions)
	}
	resendMsg := actions[1].Msg
	if mt, _ := resendMsg.Get(35); mt != "2" {
		t.Fatalf("Expected MsgType 2, got %v", mt)
	}
	if begin, _ := resendMsg.Get(7); begin != "100" {
		t.Errorf("Expected BeginSeqNo (7) to be 100, got %v", begin)
	}
	if end, _ := resendMsg.Get(16); end != "0" {
		t.Errorf("Expected EndSeqNo (16) to be 0, got %v", end)
	}
}

func TestEngine_SequenceResetRewindRejection(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.state = SessionActive
	engine.inSeqNum = 50

	// Counterparty maliciously or accidentally tries to rewind us to 10
	msg := buildMessage(t, engine, "4", map[uint16]string{34: "50", 36: "10"}, spec.SampleOptions{})
	actions := engine.OnMessage(&msg, time.Now())

	if engine.inSeqNum != 50 {
		t.Errorf("Engine dangerously allowed sequence rewind! Expected 50, got %v", engine.inSeqNum)
	}
	if len(actions) == 0 || actions[0].Type != ActionError {
		t.Errorf("Expected engine to reject the rewind attempt")
	}
}

func TestEngine_HeartbeatEchoesTestReqID(t *testing.T) {
	engine := setupEngine(t, "FIX44.xml", nil)
	engine.state = SessionActive

	msg := buildMessage(t, engine, "1", map[uint16]string{112: "ECHO_ME_123"}, spec.SampleOptions{})
	actions := engine.OnMessage(&msg, time.Now())

	if len(actions) < 1 {
		t.Fatalf("Expected actions, got %v", actions)
	}
	hbMsg := actions[0].Msg
	if mt, _ := hbMsg.Get(35); mt != "0" {
		t.Fatalf("Expected Heartbeat (0), got %v", mt)
	}
	if reqID, _ := hbMsg.Get(112); reqID != "ECHO_ME_123" {
		t.Errorf("Expected TestReqID (112) to be echoed, got %v", reqID)
	}
}
