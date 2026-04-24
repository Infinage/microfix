package spec

import (
	"slices"
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
