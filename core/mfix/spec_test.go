package mfix

import (
	"slices"
	"strings"
	"testing"
)

func TestLoadSpecs(t *testing.T) {
	// Table of expected counts to verify parsing depth
	type specStats struct {
		msgCount      int
		fieldCount    int
		headerLength  int
		trailerLength int
	}

	stats := map[string]specStats{
		"FIX40.xml":    {msgCount: 27, fieldCount: 138, headerLength: 18, trailerLength: 3},
		"FIX41.xml":    {msgCount: 28, fieldCount: 206, headerLength: 22, trailerLength: 3},
		"FIX42.xml":    {msgCount: 46, fieldCount: 405, headerLength: 27, trailerLength: 3},
		"FIX43.xml":    {msgCount: 68, fieldCount: 635, headerLength: 28, trailerLength: 3},
		"FIX44.xml":    {msgCount: 93, fieldCount: 912, headerLength: 27, trailerLength: 3},
		"FIX50.xml":    {msgCount: 93, fieldCount: 1090, headerLength: 0, trailerLength: 0},
		"FIX50SP1.xml": {msgCount: 105, fieldCount: 1372, headerLength: 0, trailerLength: 0},
		"FIX50SP2.xml": {msgCount: 108, fieldCount: 1451, headerLength: 0, trailerLength: 0},
		"FIXT11.xml":   {msgCount: 7, fieldCount: 62, headerLength: 29, trailerLength: 3},
	}

	for fileName, expected := range stats {
		t.Run(fileName, func(t *testing.T) {
			// loadRawSpec handles checking both local disk and embed.FS
			spec, err := LoadSpec(fileName)
			if err != nil {
				t.Fatalf("Failed to load %s: %v", fileName, err)
			}

			// Validate Basic Metadata
			if spec.Major == 0 {
				t.Errorf("%s: Major version parsed as 0", fileName)
			}

			// Validate Counts
			if got := len(spec.Messages); got != expected.msgCount {
				t.Errorf("%s: Expected %d messages, got %d", fileName, expected.msgCount, got)
			}

			if got := len(spec.Fields); got != expected.fieldCount {
				t.Errorf("%s: Expected %d fields, got %d", fileName, expected.fieldCount, got)
			}

			if got := len(spec.Header.Entries); got != expected.headerLength {
				t.Errorf("%s: Expected %d header entries, got %d", fileName, expected.fieldCount, got)
			}

			if got := len(spec.Trailer.Entries); got != expected.trailerLength {
				t.Errorf("%s: Expected %d trailer entries, got %d", fileName, expected.fieldCount, got)
			}
		})
	}
}

func TestLoadSpecDeepValidation(t *testing.T) {
	spec, err := LoadSpec("FIXT11.xml")
	if err != nil {
		t.Fatalf("Failed to load FIXT11: %v", err)
	}

	// Test for non-existent fields
	_, err = spec.Field(9999)
	if err == nil {
		t.Error("Expected error for non-existent tag 9999")
	}

	// Verify Field Map and Bi-directional Lookup
	// Tag 8 -> BeginString
	if name := spec.Fields[8].Name; name != "BeginString" {
		t.Errorf("Expected tag 8 to be BeginString, got %s", name)
	}
	if tag := spec.FieldNames["BeginString"]; tag != 8 {
		t.Errorf("Expected BeginString to be tag 8, got %d", tag)
	}

	// Verify Component Flattening in Header
	// In FIXT11.xml, the header has <component name='HopGrp' />
	// This should be flattened: HopGrp -> NoHops (Group)
	// Let's check if the 'NoHops' tag (627) exists in the flattened Header lookup
	hopGrpPos, found := spec.Header.Lookup[627]
	if !found {
		t.Fatal("Header should contain flattened group 'NoHops' (627) from 'HopGrp' component")
	}
	noHopsEntry := spec.Header.Entries[hopGrpPos]
	if !noHopsEntry.IsGroup {
		t.Error("NoHops entry should be marked as IsGroup")
	}

	// Verify Nested Group Lookups (Recursion)
	// NoHops (627) -> HopCompID (628)
	if _, found := noHopsEntry.Lookup[628]; !found {
		t.Error("NoHops group should have a lookup entry for HopCompID (628)")
	}

	// Verify Message Specifics: Logon (A)
	logon, ok := spec.Messages["A"]
	if !ok {
		t.Fatal("MsgType 'A' (Logon) not found in compiled spec")
	}

	// Verify required field EncryptMethod (98)
	encryptPos, found := logon.Lookup[98]
	if !found {
		t.Error("Logon should contain EncryptMethod (98)")
	}
	if !logon.Entries[encryptPos].Required {
		t.Error("EncryptMethod (98) should be required in Logon")
	}

	// Verify Enums are preserved
	msgTypeField, err := spec.Field(35)
	if err != nil {
		t.Fatal("Field 35 (MsgType) definition missing")
	}
	if msgTypeField.Name != "MsgType" || msgTypeField.Type != "STRING" {
		t.Errorf("Field 35 must have Name, Type set to 'MsgType' and 'STRING'")
	}

	foundHeartbeat := slices.ContainsFunc(msgTypeField.Enums, func(enum EnumDef) bool {
		return enum.Enum == "0" && enum.Description == "HEARTBEAT"
	})

	if !foundHeartbeat {
		t.Error("Enum values for MsgType (35) not found in compiled spec")
	}
}

// Sample API
func TestSample(t *testing.T) {
	spec, err := LoadSpec("FIXT11.xml")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("SampleRequiredOnly", func(t *testing.T) {
		msg, err := spec.Sample("A", true, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Check for count of fields
		// Header + Logon + Trailer
		if want := 7 + 3 + 1; want != len(msg) {
			t.Errorf("Expected to have %v entries, contains %v", want, len(msg))
		}

		// Check for a required field
		if _, pos := msg.Find(98); pos == -1 {
			t.Error("Required field 98 (EncryptMethod) missing from sample")
		}

		// Check that optional field is missing
		if _, pos := msg.Find(553); pos != -1 {
			t.Error("Optional field 553 (Username) should not be present in requiredOnly sample")
		}
	})

	t.Run("FlatGroupExpansion", func(t *testing.T) {
		// MsgTypeGrp has NoMsgTypes(384) which contains RefMsgType(372)
		msg, err := spec.Sample("A", false, map[uint16]int{384: 2})
		if err != nil {
			t.Fatal(err)
		}

		// Check for count of fields
		// Header + Logon + Trailer
		if want := 32 + 20 + 3; len(msg) != want {
			t.Errorf("Expected to have %v entries, contains %v", want, len(msg))
		}

		// Counter tag check
		countField, pos := msg.Find(384)
		if pos == -1 || countField.Value != "2" {
			t.Errorf("Expected NoMsgTypes(384) to be 2, got %v", countField.Value)
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
		overrides := map[uint16]int{627: 3}
		msg, err := spec.Sample("0", false, overrides) // Heartbeat (includes header)
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
		msg, err := spec.Sample("0", true, nil)
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
		msg, err := spec.Sample("A", true, nil)
		if err != nil {
			t.Fatal(err)
		}

		// EncryptMethod (98) has enums. 0 = NONE_OTHER.
		field, pos := msg.Find(98)
		if pos == -1 {
			t.Fatal("Tag 98 missing")
		}
		if field.Value != "0" {
			t.Errorf("Expected first enum value '0', got %v", field.Value)
		}
	})
}

func TestValidate_HappyPath(t *testing.T) {
	spec, _ := LoadSpec("FIXT11.xml")

	t.Run("SampleAndStrictValidate", func(t *testing.T) {
		// Generate a valid message with all optional fields
		msg, err := spec.Sample("A", false, nil)
		if err != nil {
			t.Fatalf("Failed to sample: %v", err)
		}

		// Validate with Strict mode
		ok, obs := spec.Validate(&msg, Strict)
		if !ok {
			t.Errorf("Validation failed for sampled message: %v", strings.Join(obs, "; "))
		}
	})
}

func TestValidate_CorruptedMessages(t *testing.T) {
	spec, _ := LoadSpec("FIXT11.xml")

	t.Run("InvalidChecksum", func(t *testing.T) {
		msg, _ := spec.Sample("0", true, nil)

		// Corrupt the checksum (Tag 10)
		if _, pos := msg.Find(10); pos != -1 {
			msg[pos].Value = "999"
		}

		ok, obs := spec.Validate(&msg, Basic)
		if ok {
			t.Error("Validation should have failed for bad checksum")
		}

		found := slices.ContainsFunc(obs, func(ob string) bool {
			return strings.Contains(ob, "Checksum validation failed")
		})

		if !found {
			t.Errorf("Expected checksum error in observations, got: %v", strings.Join(obs, "; "))
		}
	})

	t.Run("InvalidBodyLength", func(t *testing.T) {
		msg, _ := spec.Sample("0", true, nil)

		// Corrupt BodyLength (Tag 9)
		if _, pos := msg.Find(9); pos != -1 {
			msg[pos].Value = "0"
		}

		ok, obs := spec.Validate(&msg, Basic)
		if ok {
			t.Error("Validation should have failed for bad body length")
		}

		found := slices.ContainsFunc(obs, func(ob string) bool {
			return strings.Contains(ob, "Bodylength validation failed")
		})

		if !found {
			t.Errorf("Expected bodylength error in observations, got: %v", strings.Join(obs, "; "))
		}
	})

	t.Run("MissingRequiredField", func(t *testing.T) {
		msg, _ := spec.Sample("A", true, nil)
		ok, _ := spec.Validate(&msg, Basic)
		if !ok {
			t.Error("Validation expected to pass, but failed")
		}

		// Remove EncryptMethod (Tag 98), which is required for Logon
		corrupted := slices.DeleteFunc(msg, func(f Field) bool {
			return f.Tag == 98
		})

		// Recalculate checksum and bodylen
		spec.Finalize(&corrupted, "A")

		// It should only throw for the missing requiref field
		ok, obs := spec.Validate(&corrupted, Basic)
		if ok {
			t.Error("Validation should have failed when missing required field 98")
		}

		found := slices.ContainsFunc(obs, func(ob string) bool {
			return strings.Contains(ob, "Missing required field tag [98]")
		})

		if !found {
			t.Errorf("Expected missing field error in observations, got: %v", strings.Join(obs, "; "))
		}
	})
}

func TestValidate_DataTypeAndUnknownTags(t *testing.T) {
	spec, _ := LoadSpec("FIXT11.xml")

	t.Run("InvalidDataType_Strict", func(t *testing.T) {
		msg, _ := spec.Sample("A", true, nil)

		// HeartBtInt (Tag 108) is an INT. Let's put a string.
		if _, pos := msg.Find(108); pos != -1 {
			msg[pos].Value = "ABC"
		} else {
			t.Fatal("Missing tag from Logon [108]")
		}

		// Finalize to recalculate the checksum, bodylen
		spec.Finalize(&msg, "A")

		ok, obs := spec.Validate(&msg, Strict)
		if ok {
			t.Error("Strict validation should catch non-integer value for Tag 108")
		}

		found := slices.ContainsFunc(obs, func(ob string) bool {
			return strings.Contains(ob, "Datatype validation failed for tag [108]")
		})

		if !found {
			t.Errorf("Expected missing datatype error in observations, got: %v", strings.Join(obs, "; "))
		}
	})

	t.Run("UnknownTag_Strict", func(t *testing.T) {
		// Inject a random tag not in the spec
		msg, _ := spec.Sample("0", true, nil)
		msg = append(msg, Field{9999, "Unknown"})
		spec.Finalize(&msg, "0")

		ok, _ := spec.Validate(&msg, Strict)
		if ok {
			t.Error("Strict validation should fail for unknown tag 9999")
		}

		// Verify Basic validation ignores it (doesn't fail)
		ok, obs := spec.Validate(&msg, Basic)
		if !ok {
			t.Errorf("Basic validation should ignore unknown tags, got %v", strings.Join(obs, "; "))
		}
	})
}

func TestValidate_FromString(t *testing.T) {
	spec, err := LoadSpec("FIX44.xml")
	if err != nil {
		t.Fatal("Failed to load Spec")
	}

	t.Run("ValidRawString", func(t *testing.T) {
		var samples = []string{
			"8=FIX.4.4|9=63|35=5|34=1091|49=TESTBUY1|52=20180920-18:24:58.675|56=TESTSELL1|10=138|",
			"8=FIX.4.4|9=75|35=A|34=1092|49=TESTBUY1|52=20180920-18:24:59.643|56=TESTSELL1|98=0|108=60|10=178|"}
		for _, raw := range samples {
			msg, err := MessageFromString(raw, "|")
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			ok, obs := spec.Validate(&msg, Strict)
			if !ok {
				t.Errorf("Validation failed for raw string: %v", strings.Join(obs, "; "))
			}
		}
	})

	t.Run("InvalidRawString", func(t *testing.T) {
		var samples = []string{
			"8=FIX.4.4|9=61|35=5|34=1091|49=TESTBUY1|52=20180920-18:24:58.675|56=TESTSELL1|10=138|",
			"8=FIX.4.4|9=75|35=A|34=1092|49=TESTBUY1|52=20180920-18:24:59.643|56=TESTSELL1|98=0|108=60|10=170|"}
		for _, raw := range samples {
			msg, err := MessageFromString(raw, "|")
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			ok, _ := spec.Validate(&msg, Strict)
			if ok {
				t.Errorf("Validation expected to fail, but didn't")
			}
		}
	})
}

func TestValidate_GroupOrdering(t *testing.T) {
	spec, _ := LoadSpec("FIXT11.xml")

	// Generate a message with a group (e.g., HopGrp in Header)
	// NoHops(627) -> HopCompID(628), HopSendingTime(629), HopRefID(630)
	logon, _ := spec.Sample("0", false, map[uint16]int{627: 2})

	// Find the member tags and swap them manually to break the order
	var _, pos628 = logon.Find(628)
	var _, pos629 = logon.Find(629)
	if pos628 == -1 || pos629 == -1 {
		t.Fatal("Tag 628, 629 not found in the sampled message")
	}

	// Swap first 628 and 629, finalize not required
	t.Run("InvalidAnchorTag", func(t *testing.T) {
		msg := slices.Clone(logon)
		msg[pos628], msg[pos629] = msg[pos629], msg[pos628]
		ok, obs := spec.Validate(&msg, Strict)
		if ok {
			t.Error("Validation should fail when group members are out of order")
		} else {
			found := slices.ContainsFunc(obs, func(ob string) bool {
				return strings.Contains(ob, "Tag 629 immediately following groupno missing or not at first position")
			})
			if !found {
				t.Errorf("Expected error not found in observations, got: %v", strings.Join(obs, "; "))
			}
		}
	})

	// Swap second 628 and 629, finalize not required
	t.Run("RepeatingGroupOrderMismatch", func(t *testing.T) {
		msg := slices.Clone(logon)
		_, pos628 = msg.FindFrom(628, pos628+1)
		_, pos629 = msg.FindFrom(629, pos629+1)
		msg[pos628], msg[pos629] = msg[pos629], msg[pos628]
		ok, obs := spec.Validate(&msg, Strict)
		if ok {
			t.Error("Validation should fail when group members are out of order")
		} else {
			found := slices.ContainsFunc(obs, func(ob string) bool {
				return strings.Contains(ob, "Expected group #2 entry #1 to be 628")
			})
			if !found {
				t.Errorf("Expected error not found in observations, got: %v", strings.Join(obs, "; "))
			}
		}
	})
}
