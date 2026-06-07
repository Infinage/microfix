package spec

import "testing"

func TestSample(t *testing.T) {
	spec, err := LoadSpec("FIXT11.xml")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("SampleHeader", func(t *testing.T) {
		// SampleHeader does not return an error, just the message slice
		msg := spec.SampleHeader(SampleOptions{})

		// Check for required header tags defined in FIXT11.xml
		for _, tag := range []uint16{8, 9, 35, 49, 56, 34, 52} {
			if _, ok := msg.Get(tag); !ok {
				t.Errorf("Required Header field [%v] missing", tag)
			}
		}

		// Ensure no non-required tags are populated
		if size := len(msg); size != 7 {
			t.Errorf("Expected header to have 7 entries, got %v", size)
		}
	})

	t.Run("SampleTrailer", func(t *testing.T) {
		msg := spec.SampleTrailer(SampleOptions{})

		// Trailer in FIXT11.xml only has CheckSum (10) as required
		if _, ok := msg.Get(10); !ok {
			t.Error("Required Trailer field [10] missing")
		}

		if size := len(msg); size != 1 {
			t.Errorf("Expected trailer to have 1 entry, got %v", size)
		}
	})

	t.Run("SampleBodyRequiredOnly", func(t *testing.T) {
		msg, err := spec.SampleBody("A", SampleOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Logon (A) required body tags: EncryptMethod(98), HeartBtInt(108), DefaultApplVerID(1137)
		for _, tag := range []uint16{98, 108, 1137} {
			if _, ok := msg.Get(tag); !ok {
				t.Errorf("Required Body field [%v] missing", tag)
			}
		}

		// Ensure exactly 3 fields were generated for the body
		if size := len(msg); size != 3 {
			t.Errorf("Expected body to have 3 entries, got %v", size)
		}

		// Ensure optional fields are omitted
		if _, ok := msg.Get(553); ok {
			t.Error("Optional field 553 (Username) should not be present in requiredOnly sample")
		}
	})

	t.Run("SampleBodyFlatGroupExpansion", func(t *testing.T) {
		// MsgTypeGrp inside Logon has NoMsgTypes(384) which contains RefMsgType(372)
		msg, err := spec.SampleBody("A", SampleOptions{IncludeOptional: true, GroupOverrides: map[uint16]int{384: 2}})
		if err != nil {
			t.Fatal(err)
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

	t.Run("WhitelistOptionalFields", func(t *testing.T) {
		// Username(553) is an optional field in Logon (A)
		opts := SampleOptions{OptionalFields: map[uint16]any{553: nil}}
		msg, err := spec.SampleBody("A", opts)
		if err != nil {
			t.Fatal(err)
		}

		if user, ok := msg.Get(553); !ok {
			t.Error("Whitelisted optional field 553 (Username) is missing")
		} else if user == "" {
			t.Error("Whitelisted optional field 553 has empty value")
		}

		// Password(554) is also optional, but not whitelisted
		if _, ok := msg.Get(554); ok {
			t.Error("Non-whitelisted optional field 554 (Password) should not be present")
		}

		// EncryptMethod(98) is required, should still be there
		if _, ok := msg.Get(98); !ok {
			t.Error("Required field 98 (EncryptMethod) was incorrectly excluded by whitelist")
		}
	})
}

func TestRouterSample(t *testing.T) {
	// FIXT11 for session and FIX50 for application
	router, err := NewRouter("FIXT11.xml", []string{"FIX50.xml"})
	if err != nil {
		t.Fatal(err)
	}

	// Map to the BeginString of FIX50.xml
	if !router.SetDefaultApplVer("FIX50") {
		t.Fatal("Could not set defaultApplVer to FIX50")
	}

	t.Run("SampleAdminMessage", func(t *testing.T) {
		// Logon (A) is an Admin message. It should pull entirely from FIXT11.
		msg, err := router.Sample("A", SampleOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Verify injected tags
		if val, _ := msg.Get(8); val != "FIXT.1.1" {
			t.Errorf("Expected BeginString to be FIXT.1.1, got %v", val)
		}
		if val, _ := msg.Get(35); val != "A" {
			t.Errorf("Expected MsgType to be A, got %v", val)
		}

		// Verify an admin body tag exists (EncryptMethod)
		if _, ok := msg.Get(98); !ok {
			t.Error("Missing Logon body tag (98)")
		}

		// Verify Finalize() worked
		if _, ok := msg.Get(9); !ok {
			t.Error("Missing BodyLength (9)")
		}
		if _, ok := msg.Get(10); !ok {
			t.Error("Missing CheckSum (10)")
		}
	})

	t.Run("SampleApplicationMessage", func(t *testing.T) {
		// New Order Single (D) is an App message.
		// Header/Trailer should be FIXT11, Body should be FIX50.
		msg, err := router.Sample("D", SampleOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Verify Session Tags (from FIXT)
		if val, _ := msg.Get(8); val != "FIXT.1.1" {
			t.Errorf("Expected BeginString to be FIXT.1.1, got %v", val)
		}

		// Verify Application Tags (from FIX 5.0)
		// Tag 11 (ClOrdID) and Tag 54 (Side) are required in FIX 5.0 NOS
		for _, tag := range []uint16{11, 54} {
			if _, ok := msg.Get(tag); !ok {
				t.Errorf("Missing required FIX 5.0 New Order Single tag [%v]", tag)
			}
		}
	})

	t.Run("SampleUnknownMessage", func(t *testing.T) {
		_, err := router.Sample("ZZZ", SampleOptions{})
		if err == nil {
			t.Fatal("Expected error when sampling an unknown MsgType")
		}
	})
}
