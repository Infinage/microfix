package gui

import (
	"testing"

	"github.com/infinage/microfix/pkg/spec"
)

func Test_flattenMessageSpec(t *testing.T) {
	sp, err := spec.LoadSpec("FIX44")
	if err != nil {
		t.Fatalf("Failed to load spec: %s", err.Error())
	}

	t.Run("Sample Values check", func(t *testing.T) {
		entry, ok := sp.Messages["BE"]
		if !ok {
			t.Fatal("Missing entry [BE]")
		}

		var result []FieldInfo
		if err := flattenMessageSpec(&result, entry, &sp, false); err != nil {
			t.Errorf("Unexpected error in flattening message [V]: %s", err.Error())
		}

		expected := []FieldInfo{
			{Tag: 923, Name: "UserRequestID", Required: "Y", SampleValues: "String"},
			{Tag: 924, Name: "UserRequestType", Required: "Y", SampleValues: "Int(1=LOGONUSER,2=LOGOFFUSER,3=CHANGEPASSWORDFORUSER,4=REQUEST_INDIVIDUAL_USER_STATUS)"},
			{Tag: 553, Name: "Username", Required: "Y", SampleValues: "String"},
		}

		for pos := range 3 {
			if got, want := result[pos], expected[pos]; want != got {
				t.Errorf("Mismatch: got '%v' != want '%v'", got, want)
			}
		}
	})

	t.Run("Nested group", func(t *testing.T) {
		entry, ok := sp.Messages["V"]
		if !ok {
			t.Fatal("Missing entry [V]")
		}

		var result []FieldInfo
		if err := flattenMessageSpec(&result, entry, &sp, false); err != nil {
			t.Errorf("Unexpected error in flattening message [V]: %s", err.Error())
		}

		expected := map[int]struct {
			Tag  uint16
			Name string
		}{
			0: {Tag: 262, Name: "MDReqID"},
			3: {Tag: 267, Name: "NoMDEntryTypes"},
			4: {Tag: 269, Name: "MDEntryType"},
			5: {Tag: 146, Name: "NoRelatedSym"},
		}

		for pos, info := range result {
			if expect, ok := expected[pos]; ok {
				if info.Tag != expect.Tag || info.Name != expect.Name {
					t.Errorf("Expected '%d' entry [%d, %s], found [%d %s]",
						pos, expect.Tag, expect.Name, info.Tag, info.Name)
				}
			}
		}
	})
}
