package session

import (
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/spec"
)

func TestEngine_LogonFlow(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30)

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
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30)
	engine.OnStart(true)

	msg, _ := engine.Spec.Sample("A", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "1")
	msg.Set(108, "30")
	msg.Finalize()

	actions := engine.OnMessage(&msg, time.Now())

	if engine.State() != SessionActive {
		t.Fatalf("expected state Active")
	}
	if len(actions) != 0 {
		t.Fatalf("expected no actions, got %v", actions)
	}
}

func TestEngine_SequenceGap(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 30)
	engine.setState(SessionActive)
	engine.inSeqNum = 2

	msg, _ := engine.Spec.Sample("D", spec.SampleOptions{})
	msg.Set(49, "T")
	msg.Set(56, "S")
	msg.Set(34, "10")
	msg.Finalize()

	actions := engine.OnMessage(&msg, time.Now())

	if len(actions) == 0 || actions[0].Type != ActionSend {
		t.Fatalf("expected resend request")
	}

	mt, _ := actions[0].Msg.Get(35)
	if mt != "2" {
		t.Fatalf("expected MsgType 2, got %s", mt)
	}
}

func TestEngine_HeartbeatOnIdle(t *testing.T) {
	engine, _ := NewEngine("FIX44.xml", "S", "T", 1)
	engine.setState(SessionActive)
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
