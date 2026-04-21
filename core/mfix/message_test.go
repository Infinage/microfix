package mfix

import (
	"strings"
	"testing"
)

func TestMessage_BasicOps(t *testing.T) {
	msg := Message{
		{8, "FIX.4.4"},
		{35, "5"},
		{34, "1091"},
		{49, "TESTBUY1"},
	}

	if len(msg) != 4 {
		t.Errorf("Expected length 4, got %d", len(msg))
	}

	_, idx := msg.Find(35)
	if idx == -1 {
		t.Error("Expected to find tag 35")
	}

	_, idx = msg.Find(999)
	if idx != -1 {
		t.Error("Did not expect to find tag 999")
	}

	code, err := msg.Code()
	if err != nil || code != "5" {
		t.Errorf("Expected code 5, got %s (err: %v)", code, err)
	}
}

func TestMessage_SerializationOrdering(t *testing.T) {
	msg := Message{{5000, "1"}, {49, "A"}, {5000, "2"}}
	want := "5000=1|49=A|5000=2|"
	if got := msg.String("|"); got != want {
		t.Errorf("Got %s, want %s", got, want)
	}
}

func TestMessage_WireRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		sep  string
	}{
		{
			"Standard Heartbeat",
			"8=FIX.4.4|9=63|35=0|34=10|49=SENDER|56=TARGET|10=123|",
			"|",
		},
		{
			"Execution Report with Spaces",
			"8=FIX.4.2|35=8|55=MSFT|150=0|151=100|58=Partial Fill - Executed at limit|10=050|",
			"|",
		},
		{
			"Custom Tags and Empty Values",
			"8=FIX.4.4|35=D|5000=CustomData|1= |10=001|",
			"|",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Deserialize (Wire -> Message)
			msg, err := MessageFromString(tt.raw, tt.sep)
			if err != nil {
				t.Fatalf("Failed to parse valid wire string: %v", err)
			}

			// Serialize back (Message -> Wire)
			reserialized := msg.String(tt.sep)

			// Verify string parity
			if reserialized != tt.raw {
				t.Errorf("\nString Mismatch!\nWant: %s\nGot:  %s", tt.raw, reserialized)
			}

			// Manual Field Verification
			// This ensures the internal slice isn't just a junk buffer that happens to stringify well
			expectedFields := strings.Split(tt.raw, tt.sep)

			if len(msg) != len(expectedFields)-1 {
				t.Errorf("Field count mismatch: want %d, got %d", len(expectedFields)-1, len(msg))
			}

			for i, field := range msg {
				expectedKV := field.string()
				if expectedKV != expectedFields[i] {
					t.Errorf("Field %d mismatch: want %s, got %s", i, expectedFields[i], expectedKV)
				}
			}
		})
	}
}

func TestMessage_FindFrom(t *testing.T) {
	msg := Message{{5000, "A"}, {49, "X"}, {5000, "B"}}

	// Find first
	f1, i1 := msg.FindFrom(5000, 0)
	if i1 != 0 || f1.value != "A" {
		t.Errorf("First find failed: index %d, value %s", i1, f1.value)
	}

	// Find second
	f2, i2 := msg.FindFrom(5000, i1+1)
	if i2 != 2 || f2.value != "B" {
		t.Errorf("Second find failed: index %d, value %s", i2, f2.value)
	}
}

func TestMessage_FindAll(t *testing.T) {
	msg := Message{{5000, "A"}, {5000, "B"}, {49, "X"}}
	var values []string

	for f := range msg.FindAll(5000) {
		values = append(values, f.value)
	}

	if len(values) != 2 || values[0] != "A" || values[1] != "B" {
		t.Errorf("FindAll got %v, want %v", values, [2]string{"A", "B"})
	}
}

func TestMessageFromString_Valid(t *testing.T) {
	raw := "8=FIX.4.4|9=63|35=A|10=123|"
	msg, err := MessageFromString(raw, "|")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(msg) != 4 {
		t.Errorf("Expected 4 fields, got %d: %v", len(msg), msg)
	}

	if val, _ := msg.Code(); val != "A" {
		t.Errorf("Expected MsgType A, got %s", val)
	}
}

func TestMessageFromString_Malformed(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"Empty", ""},
		{"NoEquals", "8FIX.4.4|"},
		{"BadTag", "ABC=Value|"},
		{"MultipleEquals", "8=FIX=4.4|"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MessageFromString(tt.raw, "|")
			if err == nil {
				t.Errorf("Expected error for %s, but got nil", tt.name)
			}
		})
	}
}
