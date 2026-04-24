package message

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
				expectedKV := field.ToWire()
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
	if i1 != 0 || f1.Value != "A" {
		t.Errorf("First find failed: index %d, value %s", i1, f1.Value)
	}

	// Find second
	f2, i2 := msg.FindFrom(5000, i1+1)
	if i2 != 2 || f2.Value != "B" {
		t.Errorf("Second find failed: index %d, value %s", i2, f2.Value)
	}
}

func TestMessage_FindAll(t *testing.T) {
	msg := Message{{5000, "A"}, {5000, "B"}, {49, "X"}}
	var values []string

	for f := range msg.FindAll(5000) {
		values = append(values, f.Value)
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

func TestChecksumAndBodyLength(t *testing.T) {
	// 8=FIX.4.2|9=49|35=0|49=SENDER|56=TARGET|34=1|52=20230101-12:00:00|10=123|
	msg := Message{
		{Tag: 8, Value: "FIX.4.2"},
		{Tag: 9, Value: "49"},
		{Tag: 35, Value: "0"},
		{Tag: 49, Value: "SENDER"},
		{Tag: 56, Value: "TARGET"},
		{Tag: 34, Value: "1"},
		{Tag: 52, Value: "20230101-12:00:00"},
		{Tag: 10, Value: "123"},
	}

	t.Run("BodyLength Calculation", func(t *testing.T) {
		got := msg.BodyLength()
		expected := uint64(51)
		if got != expected {
			t.Errorf("BodyLength() = %d; want %d", got, expected)
		}
	})

	t.Run("Checksum Calculation", func(t *testing.T) {
		got := msg.Checksum()
		if got == 0 {
			t.Error("Checksum returned 0, likely failure in calculation logic")
		}
	})

	t.Run("Position Agnostic Check", func(t *testing.T) {
		// Swap 49 and 56
		msgSwapped := Message{
			{Tag: 8, Value: "FIX.4.2"},
			{Tag: 9, Value: "49"},
			{Tag: 35, Value: "0"},
			{Tag: 56, Value: "TARGET"},
			{Tag: 49, Value: "SENDER"},
			{Tag: 34, Value: "1"},
			{Tag: 52, Value: "20230101-12:00:00"},
		}

		if msg.Checksum() != msgSwapped.Checksum() {
			t.Error("Checksum changed after field swap; parser is not position-agnostic")
		}
		if msg.BodyLength() != msgSwapped.BodyLength() {
			t.Error("BodyLength changed after field swap")
		}
	})
}

func TestChecksumVerification(t *testing.T) {
	tests := []struct {
		name     string
		msg      Message
		expLen   uint64
		expCheck uint8
	}{
		{
			name: "Logon FIX 4.4",
			msg: Message{
				{Tag: 8, Value: "FIX.4.4"},
				{Tag: 9, Value: "75"},
				{Tag: 35, Value: "A"},
				{Tag: 34, Value: "1092"},
				{Tag: 49, Value: "TESTBUY1"},
				{Tag: 52, Value: "20180920-18:24:59.643"},
				{Tag: 56, Value: "TESTSELL1"},
				{Tag: 98, Value: "0"},
				{Tag: 108, Value: "60"},
			},
			expLen:   75,
			expCheck: 178,
		},
		{
			name: "Logout FIX 4.4",
			msg: Message{
				{Tag: 8, Value: "FIX.4.4"},
				{Tag: 9, Value: "63"},
				{Tag: 35, Value: "5"},
				{Tag: 34, Value: "1091"},
				{Tag: 49, Value: "TESTBUY1"},
				{Tag: 52, Value: "20180920-18:24:58.675"},
				{Tag: 56, Value: "TESTSELL1"},
			},
			expLen:   63,
			expCheck: 138,
		},
		{
			name: "Allocation FIX 4.2",
			msg: Message{
				{Tag: 8, Value: "FIX.4.2"},
				{Tag: 9, Value: "127"},
				{Tag: 35, Value: "P"},
				{Tag: 34, Value: "936"},
				{Tag: 49, Value: "TESTSELL3"},
				{Tag: 52, Value: "20260324-15:45:13.992"},
				{Tag: 56, Value: "TESTBUY3"},
				{Tag: 60, Value: "20260324-15:45:13.992"},
				{Tag: 70, Value: "3639096067028819307"},
				{Tag: 75, Value: "20230625"},
				{Tag: 87, Value: "0"},
			},
			expLen:   127,
			expCheck: 41,
		},
		{
			name: "NewOrderSingle FIX 4.2",
			msg: Message{
				{Tag: 8, Value: "FIX.4.2"},
				{Tag: 9, Value: "163"},
				{Tag: 35, Value: "D"},
				{Tag: 34, Value: "972"},
				{Tag: 49, Value: "TESTBUY3"},
				{Tag: 52, Value: "20190206-16:25:10.403"},
				{Tag: 56, Value: "TESTSELL3"},
				{Tag: 11, Value: "141636850670842269979"},
				{Tag: 21, Value: "2"},
				{Tag: 38, Value: "100"},
				{Tag: 40, Value: "1"},
				{Tag: 54, Value: "1"},
				{Tag: 55, Value: "AAPL"},
				{Tag: 60, Value: "20190206-16:25:08.968"},
				{Tag: 207, Value: "TO"},
				{Tag: 6000, Value: "TEST1234"},
			},
			expLen:   163,
			expCheck: 106,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualLen := tt.msg.BodyLength()
			if actualLen != tt.expLen {
				t.Errorf("%s: BodyLength = %d, want %d", tt.name, actualLen, tt.expLen)
			}

			actualCheck := tt.msg.Checksum()
			if actualCheck != tt.expCheck {
				t.Errorf("%s: Checksum = %d, want %d", tt.name, actualCheck, tt.expCheck)
			}
		})
	}
}
