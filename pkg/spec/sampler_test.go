package spec

import "testing"

// Sample API
func TestSample(t *testing.T) {
	spec, err := LoadSpec("FIXT11.xml")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("SampleRequiredOnly", func(t *testing.T) {
		msg, err := spec.Sample("A", SampleOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Check for count of fields
		// Header + Logon + Trailer
		if want := 7 + 3 + 1; want != len(msg) {
			t.Errorf("Expected to have %v entries, contains %v", want, len(msg))
		}

		// Check for a required field
		if _, ok := msg.Get(98); !ok {
			t.Error("Required field 98 (EncryptMethod) missing from sample")
		}

		// Check that optional field is missing
		if _, ok := msg.Get(553); ok {
			t.Error("Optional field 553 (Username) should not be present in requiredOnly sample")
		}
	})

	t.Run("FlatGroupExpansion", func(t *testing.T) {
		// MsgTypeGrp has NoMsgTypes(384) which contains RefMsgType(372)
		msg, err := spec.Sample("A", SampleOptions{IncludeOptional: true, GroupOverrides: map[uint16]int{384: 2}})
		if err != nil {
			t.Fatal(err)
		}

		// Check for count of fields
		// Header + Logon + Trailer
		if want := 32 + 20 + 3; len(msg) != want {
			t.Errorf("Expected to have %v entries, contains %v", want, len(msg))
		}

		// Counter tag check
		countField, ok := msg.Get(384)
		if !ok || countField != "2" {
			t.Errorf("Expected NoMsgTypes(384) to be 2, got %v", countField)
		}

		// Children check: RefMsgType(372) should appear twice
		occurrences := 0
		for range msg.FindAll(372) {
			occurrences++
		}
		if occurrences != 2 {
			t.Errorf("Expected 2 instances of RefMsgType(372), got %d", occurrences)
		}
	})

	t.Run("NestedGroupExpansion", func(t *testing.T) {
		// HopGrp component in Header has NoHops(627)
		// NoHops is a flat group in FIXT11, but testing the logic here:
		overrides := map[uint16]int{627: 3} // Heartbeat (includes header)
		msg, err := spec.Sample("0", SampleOptions{IncludeOptional: true, GroupOverrides: overrides})
		if err != nil {
			t.Fatal(err)
		}

		occurrences := 0
		for _, f := range msg {
			if f.Tag == 628 { // HopCompID inside NoHops
				occurrences++
			}
		}
		if occurrences != 3 {
			t.Errorf("Expected 3 instances of HopCompID(628), got %d", occurrences)
		}
	})

	t.Run("SampleOrdering", func(t *testing.T) {
		msg, err := spec.Sample("0", SampleOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// First fields should be BeginString(8), BodyLength(9), MsgType(35)
		if msg[0].Tag != 8 || msg[1].Tag != 9 || msg[2].Tag != 35 {
			t.Errorf("Header ordering incorrect. Got tags: %v, %v, %v", msg[0].Tag, msg[1].Tag, msg[2].Tag)
		}

		// Last field should be CheckSum(10)
		if msg[len(msg)-1].Tag != 10 {
			t.Errorf("Trailer ordering incorrect. Last tag: %v", msg[len(msg)-1].Tag)
		}
	})

	t.Run("SampleValues", func(t *testing.T) {
		msg, err := spec.Sample("A", SampleOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// EncryptMethod (98) has enums. 0 = NONE_OTHER.
		if encMethod, ok := msg.Get(98); !ok {
			t.Fatal("Tag 98 missing")
		} else if encMethod != "0" {
			t.Errorf("Expected first enum value '0', got %v", encMethod)
		}
	})

	t.Run("WhitelistOptionalFields", func(t *testing.T) {
		// We want a Logon (A) with only ONE specific optional field: Username (553)
		// Even if IncludeOptional is true, providing the map should restrict it.
		opts := SampleOptions{OptionalFields: map[uint16]any{553: nil}}

		msg, err := spec.Sample("A", opts)
		if err != nil {
			t.Fatal(err)
		}

		// Check that our whitelisted optional field is present
		if user, ok := msg.Get(553); !ok {
			t.Error("Whitelisted optional field 553 (Username) is missing")
		} else if user == "" {
			t.Error("Whitelisted optional field 553 has empty value")
		}

		// Check that a non-whitelisted optional field is missing
		// Tag 554 is Password (optional in Logon)
		if _, ok := msg.Get(554); ok {
			t.Error("Non-whitelisted optional field 554 (Password) should not be present")
		}

		// Ensure required fields are still there (they should ignore the whitelist)
		if _, ok := msg.Get(98); !ok {
			t.Error("Required field 98 (EncryptMethod) was incorrectly excluded by whitelist logic")
		}
	})
}
