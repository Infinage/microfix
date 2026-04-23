package mfix

import (
	"testing"
)

func TestLoadRawSpecs(t *testing.T) {
	// Table of expected counts to verify parsing depth
	type specStats struct {
		msgCount      int
		compCount     int
		fieldCount    int
		headerLength  int
		trailerLength int
	}

	stats := map[string]specStats{
		"FIX40.xml":    {msgCount: 27, compCount: 0, fieldCount: 138, headerLength: 18, trailerLength: 3},
		"FIX41.xml":    {msgCount: 28, compCount: 0, fieldCount: 206, headerLength: 22, trailerLength: 3},
		"FIX42.xml":    {msgCount: 46, compCount: 0, fieldCount: 405, headerLength: 27, trailerLength: 3},
		"FIX43.xml":    {msgCount: 68, compCount: 10, fieldCount: 635, headerLength: 28, trailerLength: 3},
		"FIX44.xml":    {msgCount: 93, compCount: 104, fieldCount: 912, headerLength: 27, trailerLength: 3},
		"FIX50.xml":    {msgCount: 93, compCount: 121, fieldCount: 1090, headerLength: 0, trailerLength: 0},
		"FIX50SP1.xml": {msgCount: 105, compCount: 163, fieldCount: 1372, headerLength: 0, trailerLength: 0},
		"FIX50SP2.xml": {msgCount: 108, compCount: 174, fieldCount: 1451, headerLength: 0, trailerLength: 0},
		"FIXT11.xml":   {msgCount: 7, compCount: 2, fieldCount: 62, headerLength: 29, trailerLength: 3},
	}

	for fileName, expected := range stats {
		t.Run(fileName, func(t *testing.T) {
			// loadRawSpec handles checking both local disk and embed.FS
			raw, err := loadRawSpec(fileName)
			if err != nil {
				t.Fatalf("Failed to load %s: %v", fileName, err)
			}

			// Validate Basic Metadata
			if raw.Major == 0 {
				t.Errorf("%s: Major version parsed as 0", fileName)
			}

			// Validate Counts
			if got := len(raw.Messages); got != expected.msgCount {
				t.Errorf("%s: Expected %d messages, got %d", fileName, expected.msgCount, got)
			}

			if got := len(raw.Components); got != expected.compCount {
				t.Errorf("%s: Expected %d components, got %d", fileName, expected.compCount, got)
			}

			if got := len(raw.Fields); got != expected.fieldCount {
				t.Errorf("%s: Expected %d fields, got %d", fileName, expected.fieldCount, got)
			}

			if got := len(raw.Header.Entries); got != expected.headerLength {
				t.Errorf("%s: Expected %d header entries, got %d", fileName, expected.fieldCount, got)
			}

			if got := len(raw.Trailer.Entries); got != expected.trailerLength {
				t.Errorf("%s: Expected %d trailer entries, got %d", fileName, expected.fieldCount, got)
			}
		})
	}
}

func TestFIXT11DeepValidation(t *testing.T) {
	raw, err := loadRawSpec("FIXT11.xml")
	if err != nil {
		t.Fatalf("Failed to load FIXT11: %v", err)
	}

	// Metadata and Root Tag
	if raw.Type != "FIXT" || raw.Major != 1 || raw.Minor != 1 {
		t.Errorf("Expected 'FIXT 1.1', got '%s %d.%d'", raw.Type, raw.Major, raw.Minor)
	}

	// Header and Entry Type Parsing
	// We check both the identity and the 'Type.Local' tag name
	if len(raw.Header.Entries) == 0 {
		t.Fatal("Header is empty")
	}

	foundField, foundComponent := false, false
	for _, entry := range raw.Header.Entries {
		switch entry.Name {
		case "BeginString":
			foundField = true
			if entry.XMLName.Local != "field" {
				t.Errorf("BeginString: expected tag 'field', got '%s'", entry.XMLName.Local)
			}
			if !bool(entry.Required) {
				t.Error("BeginString should be required")
			}
		case "HopGrp":
			foundComponent = true
			if entry.XMLName.Local != "component" {
				t.Errorf("HopGrp: expected tag 'component', got '%s'", entry.XMLName.Local)
			}
		}
	}
	if !foundField || !foundComponent {
		t.Error("Missing BeginString or HopGrp in Header")
	}

	// Message Validation: Logon (MsgType=A)
	var logon *messageDef
	for _, m := range raw.Messages {
		if m.MsgType == "A" {
			logon = &m
			break
		}
	}
	if logon == nil {
		t.Fatal("Logon message (Type A) not found")
	}

	// Check required fields in Logon
	foundEncrypt := false
	for _, e := range logon.Entries {
		if e.Name == "EncryptMethod" && bool(e.Required) {
			foundEncrypt = true
			break
		}
	}
	if !foundEncrypt {
		t.Error("Logon message missing required field: EncryptMethod")
	}

	// Nested Component & Group Validation (HopGrp -> NoHops -> HopCompID)
	var hopGrp *componentDef
	for _, c := range raw.Components {
		if c.Name == "HopGrp" {
			hopGrp = &c
			break
		}
	}
	if hopGrp == nil || len(hopGrp.Entries) == 0 {
		t.Fatal("Component HopGrp not found or empty")
	}

	// Verify the 'group' tag
	groupEntry := hopGrp.Entries[0]
	if groupEntry.Name != "NoHops" || groupEntry.XMLName.Local != "group" {
		t.Errorf("Expected group 'NoHops', got '%s' with tag '%s'", groupEntry.Name, groupEntry.XMLName.Local)
	}

	// Verify nested 'field' inside group
	if len(groupEntry.Entries) == 0 || groupEntry.Entries[0].Name != "HopCompID" || groupEntry.Entries[0].XMLName.Local != "field" {
		t.Errorf("NoHops group missing nested field 'HopCompID' or tag type is wrong")
	}

	// Field Definitions and Enums: MsgType (Tag 35)
	var msgTypeField *FieldDef
	for _, f := range raw.Fields {
		if f.Number == 35 {
			msgTypeField = &f
			break
		}
	}
	if msgTypeField == nil {
		t.Fatal("Field 35 (MsgType) definition missing")
	}
	if msgTypeField.Name != "MsgType" || msgTypeField.Type != "STRING" {
		t.Errorf("Field 35 must have Name, Type set to 'MsgType' and 'STRING'")
	}

	foundHeartbeat := false
	for _, v := range msgTypeField.Enums {
		if v.Enum == "0" && v.Description == "HEARTBEAT" {
			foundHeartbeat = true
			break
		}
	}
	if !foundHeartbeat {
		t.Error("Field 35 missing HEARTBEAT enum")
	}
}
