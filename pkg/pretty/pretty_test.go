package pretty

import (
	"bytes"
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

// Helper to build a minimal FIX spec for testing
func mockSpec() *spec.Spec {
	return &spec.Spec{
		Type: "MFIX", Major: 0, Minor: 1,
		FieldNames: map[string]uint16{
			"BeginString": 8, "BodyLength": 9,
			"CheckSum": 10, "MsgType": 35,
			"NoHops": 627, "HopCompID": 628,
		},
		Fields: map[uint16]spec.FieldDef{
			8:   {Name: "BeginString", Type: "STRING"},
			9:   {Name: "BodyLength", Type: "LENGTH"},
			10:  {Name: "CheckSum", Type: "STRING"},
			35:  {Name: "MsgType", Type: "STRING"},
			627: {Name: "NoHops", Type: "NUMINGROUP"},
			628: {Name: "HopCompID", Type: "STRING"},
		},
		Header: spec.Entry{
			Entries: []spec.Entry{
				{Name: "BeginString", Required: true},
				{Name: "BodyLength", Required: true},
				{Name: "MsgType", Required: true},
			},
			Lookup: map[uint16]int{8: 0, 9: 1, 35: 2},
		},
		Trailer: spec.Entry{Entries: []spec.Entry{{Name: "CheckSum", Required: true}}, Lookup: map[uint16]int{10: 0}},
		Messages: map[string]spec.Entry{
			"A": {
				Name: "Logon",
				Entries: []spec.Entry{
					{
						Name:     "NoHops",
						Required: true,
						IsGroup:  true,
						Entries:  []spec.Entry{{Name: "HopCompID", Required: true}},
						Lookup:   map[uint16]int{628: 0},
					},
				},
				Lookup: map[uint16]int{627: 0},
			},
		},
	}
}

func TestWritePrettyFieldDef(t *testing.T) {
	field := spec.FieldDef{
		Name: "MsgType",
		Type: "STRING",
		Enums: []spec.EnumDef{
			{Enum: "0", Description: "HEARTBEAT"},
			{Enum: "A", Description: "LOGON"},
		},
	}

	// Print into buffer
	var buf bytes.Buffer
	FieldDef(&buf, field)
	output := buf.String()

	// Verify header
	if !strings.Contains(output, "Field: MsgType") {
		t.Errorf("Expected output to contain field name, got:\n%s", output)
	}

	// Verify Enums (Check for exact formatting)
	if !strings.Contains(output, "  0     -> HEARTBEAT") {
		t.Errorf("Enum formatting mismatch, got:\n%s", output)
	}
}

func TestMessage_StructuredOutput(t *testing.T) {
	sp := mockSpec()
	msg, err := message.MessageFromString("8=MFIX.0.1|9=22|35=A|627=1|628=STRING|10=236|", "|")
	if err != nil {
		t.Fatal("Failed to parse mock fix string")
	}

	var buf bytes.Buffer
	err = Message(&buf, &msg, sp)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	output := buf.String()

	// Verify sections
	if !strings.Contains(output, "[HEADER]") || !strings.Contains(output, "[BODY]") {
		t.Errorf("Missing expected sections in output:\n%s", output)
	}

	// Verify group parsing visually
	if !strings.Contains(output, "Group 1") {
		t.Errorf("Failed to parse/print repeating group:\n%s", output)
	}
}

func TestMessage_FallbackOutputWithoutMsgType(t *testing.T) {
	sp := mockSpec()
	// Missing tag 35
	msg := &message.Message{
		{Tag: 8, Value: "FIX.4.4"},
		{Tag: 627, Value: "1"},
	}

	var buf bytes.Buffer
	err := Message(&buf, msg, sp)

	if err == nil {
		t.Error("Expected an error for unknown MsgType, got nil")
	}

	output := buf.String()

	// It should fallback to printFields, so sections shouldn't exist
	if strings.Contains(output, "[HEADER]") {
		t.Errorf("Expected flat output, but found structured headers:\n%s", output)
	}

	// But the fields should still be printed
	if !strings.Contains(output, "BeginString") || !strings.Contains(output, "NoHops") {
		t.Errorf("Missing expected fields in flat output:\n%s", output)
	}
}

func TestPrettyMessage_MultipleGroups(t *testing.T) {
	raw := "8=FIX.4.4|9=120|35=V|49=SENDER|56=TARGET|34=1|52=20260404-12:00:00.000|" +
		"262=REQ1|146=2|55=AAPL|55=GOOG|10=000|"

	msg, err := message.MessageFromString(raw, "|")
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	sp, err := spec.LoadSpec("FIX44.xml")
	if err != nil {
		t.Fatalf("failed to load spec: %v", err)
	}

	var buf bytes.Buffer
	err = Message(&buf, &msg, &sp)
	if err != nil {
		t.Fatalf("pretty print failed: %v", err)
	}

	out := buf.String()

	// Group count
	if !strings.Contains(out, "146") || !strings.Contains(out, "NoRelatedSym") {
		t.Errorf("missing group header in output:\n%s", out)
	}

	// Group 1 has AAPL
	if !strings.Contains(out, "Group 1") || !strings.Contains(out, "AAPL") {
		t.Errorf("Group 1 not printed correctly:\n%s", out)
	}

	// Group 2 has GOOG
	if !strings.Contains(out, "Group 2") || !strings.Contains(out, "GOOG") {
		t.Errorf("Group 2 not printed correctly:\n%s", out)
	}

	// Critical: ensure AAPL and GOOG are NOT in same group
	group1Idx := strings.Index(out, "Group 1")
	group2Idx := strings.Index(out, "Group 2")

	aaplIdx := strings.Index(out, "AAPL")
	googIdx := strings.Index(out, "GOOG")

	if !(group1Idx < aaplIdx && aaplIdx < group2Idx) {
		t.Errorf("AAPL should belong to Group 1:\n%s", out)
	}

	if !(group2Idx < googIdx) {
		t.Errorf("GOOG should belong to Group 2:\n%s", out)
	}
}
