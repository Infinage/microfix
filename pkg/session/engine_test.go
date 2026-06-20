package session

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

var (
	// Store our parsed routers globally in memory for the tests
	testRouterFIX44  *spec.Router
	testRouterFIXT11 *spec.Router

	// Ensures we only parse the XML files once, no matter how many tests run
	parseOnce sync.Once
)

// setupEngine creates a test Engine using pre-parsed routers
func setupEngine(t *testing.T, loadFIXT bool) *Engine {
	t.Helper()

	// Parse the XML files exactly ONCE for the entire test suite
	parseOnce.Do(func() {
		var err error
		testRouterFIX44, err = spec.NewDefaultRouter("FIX44.xml")
		if err != nil {
			panic("Failed to parse FIX44.xml for tests: " + err.Error())
		}

		// Here is where you explicitly load FIXT11 with a specific app spec (FIX44)
		testRouterFIXT11, err = spec.NewRouter("FIXT11.xml", []string{"FIX44.xml"})
		if err != nil {
			panic("Failed to parse FIXT11.xml for tests: " + err.Error())
		}
	})

	// Select the correct pre-parsed router from memory
	var router *spec.Router
	if loadFIXT {
		router = testRouterFIXT11
	} else {
		router = testRouterFIX44
	}

	// Pass the memory pointer to the engine
	engine, err := NewEngine(router, "S", "T", 30, EngineOptions{})
	if err != nil {
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
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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

	// Building a logon with SeqNo != 1 and ResetSeqNumFlag set
	msg = buildMessage(t, engine, "A", map[uint16]string{141: "Y", 34: "10"}, spec.SampleOptions{OptionalFields: map[uint16]any{141: nil}})
	actions = engine.OnMessage(&msg, time.Now())
	if engine.state != SessionListening {
		t.Fatal("Expected state Listening")
	} else if len(actions) != 3 || actions[0].Type != ActionError || actions[1].Type != ActionSend || actions[2].Type != ActionClose {
		t.Fatal("Expected a Error event followed by a logout")
	} else if msg, _ := actions[1].Msg.Get(58); strings.Contains(msg, "Must have MsgSeqNum set to 1") {
		t.Fatalf("Expected Logout with reason: 'Must have MsgSeqNum set to 1', got %v", msg)
	}

	// Building a logon, purposefully setting MsgSeqNum to a higher num
	msg = buildMessage(t, engine, "A", map[uint16]string{34: "10"}, spec.SampleOptions{})

	// Engine should sync InSeqNum before sending a logon response
	actions = engine.OnMessage(&msg, time.Now())
	if engine.state != SessionActive {
		t.Errorf("Expected state Active, got %v", engine.state)
	} else if len(actions) != 3 || actions[0].Type != ActionLog || actions[1].Type != ActionLog || actions[2].Type != ActionSend {
		t.Error("Expected engine to log state transition and send logon response back")
	} else if !strings.HasPrefix(actions[0].Event, "Logon InSeq [34] higher than expected") {
		t.Errorf("Expected MsgSeqNum sync log, got %v", actions[0])
	} else if msgType, _ := actions[2].Msg.Get(35); msgType != "A" {
		t.Errorf("Expected MsgType logon, got %v", msgType)
	}
}

func TestEngine_HandleLogonResponse(t *testing.T) {
	engine := setupEngine(t, false)
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

func TestEngine_HeartbeatOnIdle(t *testing.T) {
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, true)
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

		obs, ok := engine.validateAgainstSpec(&msg, spec.ValidationBasic)
		if ok {
			t.Error("Expected validateAgainstSpec to fail due to invalid ApplVerID, but passed")
		} else if len(obs) != 1 || !strings.HasPrefix(obs[0], "Message specifies unsupported ApplVerID [1128]") {
			t.Errorf("Expected ApplVerID invalid err, got: %v", obs)
		}
	})
}

func TestEngine_StrictInboundValidation(t *testing.T) {
	engine := setupEngine(t, false)

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
		if err == nil || !strings.Contains(err.Error(), "BeginString mismatch") {
			t.Fatalf("expected BeginString mismatch error, got %v", err)
		}
	})
}

func TestEngine_OutboundSequenceIncrementRules(t *testing.T) {
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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

func TestEngine_SequenceGap(t *testing.T) {
	engine := setupEngine(t, false)
	engine.state = SessionActive
	engine.inSeqNum = 2

	// Expecting inSeqNum 2 but we set SeqNum as 10
	msg := buildMessage(t, engine, "D", map[uint16]string{34: "10"}, spec.SampleOptions{})
	actions := engine.OnMessage(&msg, time.Now())

	if len(actions) != 3 || actions[1].Type != ActionSend {
		t.Fatalf("expected resend request")
	}

	mt, _ := actions[1].Msg.Get(35)
	if mt != "2" {
		t.Fatalf("expected MsgType 2, got %s", mt)
	}
}

func TestEngine_ResendRequestBuildsCorrectTags(t *testing.T) {
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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
	engine := setupEngine(t, false)
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

func TestEngine_HandleResend(t *testing.T) {
	engine := setupEngine(t, false)
	engine.state = SessionActive

	// Simulate engine state:
	// We sent 5 messages.
	// Seq 1: App
	// Seq 2: App
	// Seq 3: Admin (Heartbeat - NOT stored)
	// Seq 4: App
	// Seq 5: Admin (TestRequest - NOT stored)
	// engine.outSeqNum is currently 6 (the next message we will send)
	engine.outSeqNum = 6

	msg1 := buildMessage(t, engine, "D", map[uint16]string{34: "1"}, spec.SampleOptions{})
	msg2 := buildMessage(t, engine, "D", map[uint16]string{34: "2"}, spec.SampleOptions{})
	msg4 := buildMessage(t, engine, "D", map[uint16]string{34: "4"}, spec.SampleOptions{})

	engine.store.Append(msg1)
	engine.store.Append(msg2)
	engine.store.Append(msg4)

	t.Run("Pure GapFill (Resending only missing admin messages)", func(t *testing.T) {
		// Counterparty asks for 3 to 3. We don't have 3.
		req := buildMessage(t, engine, "2", map[uint16]string{34: "1", 7: "3", 16: "3"}, spec.SampleOptions{})
		actions := engine.OnMessage(&req, time.Now())

		if len(actions) != 1 {
			t.Fatalf("Expected 1 action, got %d", len(actions))
		}

		gapFill := actions[0].Msg
		mt, _ := gapFill.Get(35)
		gfFlag, _ := gapFill.Get(123)
		tag34, _ := gapFill.Get(34)
		tag36, _ := gapFill.Get(36)

		if mt != "4" || gfFlag != "Y" {
			t.Errorf("Expected GapFill (35=4, 123=Y), got %s, %s", mt, gfFlag)
		}
		if tag34 != "3" || tag36 != "4" {
			t.Errorf("Expected GapFill from 3 to 4, got %s to %s", tag34, tag36)
		}
	})

	t.Run("Mix of App Replay and GapFills", func(t *testing.T) {
		// Counterparty asks for 2 to 4
		// Expect: Replay 2, GapFill 3, Replay 4
		req := buildMessage(t, engine, "2", map[uint16]string{34: "2", 7: "2", 16: "4"}, spec.SampleOptions{})
		actions := engine.OnMessage(&req, time.Now())

		if len(actions) != 3 {
			t.Fatalf("Expected 3 actions, got %d", len(actions))
		}

		// Check Msg 2 Replay
		mt1, _ := actions[0].Msg.Get(35)
		pd1, _ := actions[0].Msg.Get(43)
		if mt1 != "D" || pd1 != "Y" {
			t.Errorf("Expected Replay 'D' with PossDup='Y', got '%s', '%s'", mt1, pd1)
		}

		// Check GapFill for Msg 3
		mt2, _ := actions[1].Msg.Get(35)
		tag36, _ := actions[1].Msg.Get(36)
		if mt2 != "4" || tag36 != "4" {
			t.Errorf("Expected GapFill to sequence 4, got '%s', '%s'", mt2, tag36)
		}

		// Check Msg 4 Replay
		mt3, _ := actions[2].Msg.Get(35)
		if mt3 != "D" {
			t.Errorf("Expected Replay 'D', got '%s'", mt3)
		}
	})

	t.Run("Infinity Request (16=0) with trailing gap", func(t *testing.T) {
		// Counterparty asks for 4 to 0 (Infinity)
		// Expect: Replay 4, GapFill 5 to 6 (since outSeqNum is 6)
		req := buildMessage(t, engine, "2", map[uint16]string{34: "3", 7: "4", 16: "0"}, spec.SampleOptions{})
		actions := engine.OnMessage(&req, time.Now())

		if len(actions) != 2 {
			t.Fatalf("Expected 2 actions, got %d", len(actions))
		}

		// Check Msg 4 Replay
		mt1, _ := actions[0].Msg.Get(35)
		if mt1 != "D" {
			t.Errorf("Expected Replay 'D', got %s", mt1)
		}

		// Check Trailing GapFill for Msg 5
		mt2, _ := actions[1].Msg.Get(35)
		tag34, _ := actions[1].Msg.Get(34)
		tag36, _ := actions[1].Msg.Get(36)

		if mt2 != "4" {
			t.Errorf("Expected Trailing GapFill (4), got %s", mt2)
		}
		if tag34 != "5" || tag36 != "6" {
			t.Errorf("Expected final GapFill from 5 to 6, got %s to %s", tag34, tag36)
		}
	})

	t.Run("Deep Copy Verification", func(t *testing.T) {
		// Verify that playing back the messages didn't permanently corrupt the store with 43=Y
		entries := engine.store.Fetch(1, 1)
		if len(entries) == 1 {
			if _, hasPossDup := entries[0].Msg.Get(43); hasPossDup {
				t.Error("CRITICAL: Message in store was mutated with 43=Y during replay!")
			}
		}
	})
}

func TestEngine_OutOfSyncStateAndRecovery(t *testing.T) {
	engine := setupEngine(t, false)
	engine.state = SessionActive
	engine.inSeqNum = 2
	engine.outSeqNum = 2

	t.Run("Trigger OutOfSync and verify ResendRequest", func(t *testing.T) {
		// Engine expects 2, but receives 5
		msg := buildMessage(t, engine, "D", map[uint16]string{34: "5"}, spec.SampleOptions{})
		actions := engine.OnMessage(&msg, time.Now())

		if engine.state != SessionOutOfSync {
			t.Fatalf("Expected state SessionOutOfSync, got %v", engine.state)
		} else if engine.outOfSyncUntil != 5 {
			t.Fatalf("Expected outOfSyncUntil to be 5, got %v", engine.outOfSyncUntil)
		} else if len(actions) != 3 || actions[1].Type != ActionSend {
			t.Fatalf("Expected ActionSend in generated actions, got %v", actions)
		}

		// Check if msg is a ResendRequest and is as expected
		mt, _ := actions[1].Msg.Get(35)
		tag7, _ := actions[1].Msg.Get(7)
		tag16, _ := actions[1].Msg.Get(16)
		if mt != "2" || tag7 != "2" || tag16 != "0" {
			t.Errorf("Expected ResendRequest 2 to 0, got %s to %s", tag7, tag16)
		}
	})

	t.Run("Resend Storm Prevention (Drop subsequent msgs)", func(t *testing.T) {
		// Still expecting 2. Counterparty sends 6 before our ResendRequest reaches them.
		msg := buildMessage(t, engine, "D", map[uint16]string{34: "6"}, spec.SampleOptions{})
		actions := engine.OnMessage(&msg, time.Now())
		if engine.state != SessionOutOfSync {
			t.Fatalf("Expected state SessionOutOfSync, got %v", engine.state)
		}

		// Should just log a drop, NOT send another ResendRequest
		for _, a := range actions {
			if a.Type == ActionSend {
				t.Fatal("Expected no messages to be sent (Resend Storm prevented)")
			} else if a.Type == ActionLog && !strings.Contains(a.Event, "Dropped msg 6") {
				t.Errorf("Expected drop log, got %s", a.Event)
			}
		}
	})

	t.Run("Honor incoming ResendRequests", func(t *testing.T) {
		// Populate store so we have something to replay
		engine.store.Append(buildMessage(t, engine, "D", map[uint16]string{34: "1"}, spec.SampleOptions{}))
		engine.outSeqNum = 2

		// While OutOfSync, counterparty asks us for a replay (1 to 0)
		req := buildMessage(t, engine, "2", map[uint16]string{34: "7", 7: "1", 16: "0"}, spec.SampleOptions{})
		actions := engine.OnMessage(&req, time.Now())

		// Ensure engine stays out of sync
		if engine.state != SessionOutOfSync {
			t.Fatalf("Expected to remain in SessionOutOfSync, got %v", engine.state)
		}

		// Should honor the request (send GapFill or Replay)
		if !slices.ContainsFunc(actions, func(a Action) bool { return a.Type == ActionSend }) {
			t.Fatal("Expected engine to honor ResendRequest and send messages")
		}
	})

	t.Run("Message Recovery and State Heal", func(t *testing.T) {
		// We are waiting for 2, 3, and 4. outOfSyncUntil is 5.
		msg := buildMessage(t, engine, "D", map[uint16]string{43: "Y"}, spec.SampleOptions{OptionalFields: map[uint16]any{43: nil}})
		for _, msgSeq := range []string{"2", "3"} {
			msg.Set(34, msgSeq)
			msg.Finalize()
			engine.OnMessage(&msg, time.Now())
			if engine.state != SessionOutOfSync {
				t.Fatalf("Expected to remain OutOfSync after msg %s, got %v", msgSeq, engine.state)
			}
		}

		// Session should revive when it sees SeqNum = 4, i.e. outOfSyncUntil - 1
		msg.Set(34, "4")
		msg.Finalize()
		t.Log(engine.OnMessage(&msg, time.Now()))
		if engine.state != SessionActive {
			t.Fatalf("Expected state to heal to SessionActive after msg 4, got %v", engine.state)
		} else if engine.inSeqNum != 5 {
			t.Fatalf("Expected inSeqNum to increment to 5, got %d", engine.inSeqNum)
		}
	})

	t.Run("Hard Reset heals OutOfSync", func(t *testing.T) {
		// Force engine back into a broken state
		engine.state = SessionOutOfSync
		engine.inSeqNum = 5
		engine.outOfSyncUntil = 10

		// Counterparty gives up and hard resets us to 10 (35=4, 123=N)
		msg := buildMessage(t, engine, "4", map[uint16]string{34: "10", 36: "10", 123: "N"}, spec.SampleOptions{OptionalFields: map[uint16]any{123: nil}})
		engine.OnMessage(&msg, time.Now())

		if engine.state != SessionActive {
			t.Fatalf("Expected state to heal to SessionActive on Hard Reset, got %v", engine.state)
		} else if engine.inSeqNum != 10 {
			t.Fatalf("Expected inSeqNum to reset to 10, got %d", engine.inSeqNum)
		}
	})
}
