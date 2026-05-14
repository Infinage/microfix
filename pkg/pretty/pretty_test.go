package pretty

import (
	"bytes"
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

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
	ro, err := spec.NewDefaultRouter("FIX44.xml")
	if err != nil {
		t.Fatalf("Failed to load router: %v", err)
	}

	// A valid FIX4.4 Logon Message
	msg, err := message.MessageFromString("8=FIX.4.4|9=27|35=A|108=30|98=0|10=062|", "|")
	if err != nil {
		t.Fatal("Failed to parse mock fix string")
	}

	var buf bytes.Buffer
	err = Message(&buf, &msg, ro)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	output := buf.String()

	// Verify sections
	if !strings.Contains(output, "[HEADER]") || !strings.Contains(output, "[BODY]") {
		t.Errorf("Missing expected sections in output:\n%s", output)
	}
}

func TestMessage_FallbackOutputWithoutMsgType(t *testing.T) {
	ro, err := spec.NewDefaultRouter("FIX44.xml")
	if err != nil {
		t.Fatalf("Failed to load router: %v", err)
	}

	// Missing tag 35 (MsgType)
	msg := message.Message{
		{Tag: 8, Value: "FIX.4.4"},
		{Tag: 108, Value: "30"},
	}

	var buf bytes.Buffer
	err = Message(&buf, &msg, ro)

	if err == nil {
		t.Error("Expected an error for unknown MsgType, got nil")
	}

	output := buf.String()

	// It should fallback to flat output (printFields), so sections shouldn't exist
	if strings.Contains(output, "[HEADER]") {
		t.Errorf("Expected flat output, but found structured headers:\n%s", output)
	}

	// But the fields should still be printed
	if !strings.Contains(output, "BeginString") || !strings.Contains(output, "HeartBtInt") {
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

	ro, err := spec.NewDefaultRouter("FIX44.xml")
	if err != nil {
		t.Fatalf("failed to load router: %v", err)
	}

	var buf bytes.Buffer
	err = Message(&buf, &msg, ro)
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

func TestPrettyMessage_FIXTMultiplexing(t *testing.T) {
	ro, err := spec.NewRouter("FIXT11.xml", []string{"FIX44.xml"})
	if err != nil {
		t.Fatalf("Failed to load router: %v", err)
	}

	msg, err := ro.Sample("AE", spec.SampleOptions{})
	if err != nil {
		t.Fatalf("Failed to sample message [AE]: %v", err)
	}

	var buf bytes.Buffer
	err = Message(&buf, &msg, ro)
	if err != nil {
		t.Fatalf("Pretty print failed: %v", err)
	}

	out := buf.String()

	// Every tag should be accounted for and no section should be empty
	if strings.Contains(out, "(empty)") {
		t.Errorf("Output message contains empty sections: %v", out)
	}
	if strings.Contains(out, "UNKNOWN") {
		t.Errorf("Output message contains unresolved tags: %v", out)
	}
}
