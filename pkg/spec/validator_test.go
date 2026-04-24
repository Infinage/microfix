package spec

import (
	"slices"
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

func TestDefaultStringsAreValid(t *testing.T) {
	// We iterate through all known FIX types to ensure our defaults pass our own validator
	types := []string{
		"int", "seqnum", "tagnum", "length", "numingroup",
		"amt", "float", "percentage", "price", "priceoffset", "qty",
		"boolean", "char", "multiplecharvalue", "multiplestringvalue",
		"multiplevaluestring", "utcdateonly", "localmktdate", "date",
		"utctimeonly", "localmkttime", "time", "utctimestamp", "utcdate",
		"tztimestamp", "tztimeonly", "monthyear",
	}

	for _, dtype := range types {
		t.Run(dtype, func(t *testing.T) {
			val := defaultString(dtype)
			err := validateDtype(message.Field{Value: val}, dtype)
			if err != nil {
				t.Errorf("Type [%s] has invalid default value [%s]: %v", dtype, val, err)
			}
		})
	}
}

func TestValidateDtypeEdges(t *testing.T) {
	tests := []struct {
		name    string
		val     string
		dtype   string
		wantErr bool
	}{
		{"Valid Int", "123", "int", false},
		{"Invalid Int", "abc", "int", true},
		{"Valid Float", "123.45", "price", false},
		{"Invalid Float", "12.3.4", "price", true},
		{"Valid Bool", "Y", "boolean", false},
		{"Invalid Bool", "Maybe", "boolean", true},
		{"Valid Timestamp", "20260404-12:00:00Z", "utctimestamp", false},
		{"Empty String", "", "int", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := message.Field{Value: tt.val}
			err := validateDtype(field, tt.dtype)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDtype() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_HappyPath(t *testing.T) {
	spec, err := LoadSpec("FIXT11.xml")
	if err != nil {
		t.Fatal("Failed to load spec")
	}

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
		if !msg.Set(10, "999") {
			t.Error("Checksum tag [10] missing in sampled Heartbeat")
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
		if !msg.Set(9, "0") {
			t.Error("BodyLength tag [9] missing in sampled Heartbeat")
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
		corrupted := slices.DeleteFunc(msg, func(f message.Field) bool {
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
		if !msg.Set(108, "ABC") {
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
		msg = append(msg, message.Field{Tag: 9999, Value: "Unknown"})
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
			msg, err := message.MessageFromString(raw, "|")
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
			msg, err := message.MessageFromString(raw, "|")
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
	var _, pos628 = logon.FindFrom(628, 0)
	var _, pos629 = logon.FindFrom(629, 0)
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
