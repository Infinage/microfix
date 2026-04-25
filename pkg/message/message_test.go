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

	if code, ok := msg.Get(35); !ok || code != "5" {
		t.Errorf("MsgType tag [35] missing or incorrectly set. Want '5', got '%s'", code)
	}

	if _, ok := msg.Get(999); ok {
		t.Error("Did not expect to find tag 999")
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

func TestMessage_ModifyViaFind(t *testing.T) {
	t.Run("Modify via FindFrom", func(t *testing.T) {
		msg := Message{{1, "Original"}, {2, "Stay"}}

		f, i := msg.FindFrom(1, 0)
		if i == -1 {
			t.Fatal("Tag 1 not found")
		}

		// Direct modification of the pointed-to field
		f.Value = "Modified"

		// Verify via Get
		val, _ := msg.Get(1)
		if val != "Modified" {
			t.Errorf("Expected Modified, got %s", val)
		}
	})

	t.Run("Bulk Modify via FindAll", func(t *testing.T) {
		// Scenario: Update all 'PartyRole' tags in one pass
		msg := Message{
			{452, "1"},
			{55, "MSFT"},
			{452, "2"},
		}

		// Use the iterator to modify every instance of tag 452
		for f := range msg.FindAll(452) {
			f.Value = "99"
		}

		// Verify modification
		if msg[0].Value != "99" || msg[2].Value != "99" {
			t.Errorf("Tag 452 not modified, got %s,%s", msg[0].Value, msg[1].Value)
		}
	})
}

func TestMessage_Set(t *testing.T) {
	msg := Message{
		{8, "FIX.4.4"},
		{35, "A"},
		{34, "1"},
		{49, "OLD_SENDER"},
		{34, "2"}, // Duplicate tag to test "first match" logic
	}

	t.Run("Successful Update", func(t *testing.T) {
		if ok := msg.Set(49, "NEW_SENDER"); !ok {
			t.Error("Set returned false for existing tag 49")
		}
		if val, _ := msg.Get(49); val != "NEW_SENDER" {
			t.Errorf("Expected NEW_SENDER, got %s", val)
		}
	})

	t.Run("Missing Tag", func(t *testing.T) {
		if ok := msg.Set(999, "VOID"); ok {
			t.Error("Set returned true for non-existent tag 999")
		}
	})

	t.Run("Modify First Match Only", func(t *testing.T) {
		// Tag 34 exists twice. Set should only touch the first one.
		msg.Set(34, "100")

		if msg[2].Value != "100" {
			t.Errorf("First instance of 34 not updated. Got %s", msg[2].Value)
		}
		if msg[4].Value != "2" {
			t.Errorf("Second instance of 34 was incorrectly updated. Got %s", msg[4].Value)
		}
	})
}

func TestMessage_Insert(t *testing.T) {
	t.Run("Insert at Start", func(t *testing.T) {
		msg := Message{{Tag: 35, Value: "A"}}
		msg.Insert(0, Field{Tag: 8, Value: "FIX.4.4"})

		if msg[0].Tag != 8 || msg[1].Tag != 35 {
			t.Errorf("Expected [8, 35], got [%d, %d]", msg[0].Tag, msg[1].Tag)
		}
	})

	t.Run("Insert in Middle", func(t *testing.T) {
		msg := Message{{Tag: 8, Value: "FIX.4.4"}, {Tag: 35, Value: "A"}}
		msg.Insert(1, Field{Tag: 9, Value: "100"})
		if msg[1].Tag != 9 {
			t.Errorf("Expected tag 9 at index 1, got %d", msg[1].Tag)
		}
	})

	t.Run("Insert Out of Bounds (Append)", func(t *testing.T) {
		msg := Message{{Tag: 8, Value: "FIX.4.4"}}
		msg.Insert(99, Field{Tag: 10, Value: "123"})
		if msg[1].Tag != 10 {
			t.Errorf("Expected tag 10 to be appended at end, got %d", msg[1].Tag)
		}
	})
}

func TestMessage_Contains(t *testing.T) {
	msg := &Message{
		{8, "FIX.4.4"},
		{35, "D"},
		{55, "AAPL"},
		{54, "1"},
	}

	t.Run("All Tags Present", func(t *testing.T) {
		if !msg.Contains(55, 54) {
			t.Error("Expected true for tags 55 and 54")
		}
	})

	t.Run("Single Tag Present", func(t *testing.T) {
		if !msg.Contains(8) {
			t.Error("Expected true for tag 8")
		}
	})

	t.Run("One Tag Missing", func(t *testing.T) {
		if msg.Contains(55, 99) {
			t.Error("Expected false because tag 99 is missing")
		}
	})

	t.Run("All Tags Missing", func(t *testing.T) {
		if msg.Contains(100, 101, 102) {
			t.Error("Expected false for completely missing tags")
		}
	})

	t.Run("Empty Input", func(t *testing.T) {
		if !msg.Contains() {
			t.Error("Expected true for an empty variadic input")
		}
	})
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

	if val, _ := msg.Get(35); val != "A" {
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

func TestMessage_Finalize(t *testing.T) {
	// 8=FIX.4.2 | 35=A | 49=SENDER | 56=TARGET
	msg := &Message{
		{Tag: 8, Value: "FIX.4.2"},
		{Tag: 35, Value: "A"},
		{Tag: 49, Value: "SENDER"},
		{Tag: 56, Value: "TARGET"},
	}

	msg.Finalize()

	// Assert BodyLength (Tag 9) is at index 1
	if val, _ := msg.Get(9); val == "" {
		t.Error("Tag 9 (BodyLength) missing")
	}

	// Assert Checksum (Tag 10) is at the end
	lastField := (*msg)[len(*msg)-1]
	if lastField.Tag != 10 {
		t.Errorf("Tag 10 (Checksum) should be last, got tag %d", lastField.Tag)
	}

	// Verify Checksum format (must be 3 digits, e.g., "042")
	if len(lastField.Value) != 3 {
		t.Errorf("Checksum should be 3 digits, got %s", lastField.Value)
	}
}
