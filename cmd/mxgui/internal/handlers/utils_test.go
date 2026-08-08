package gui

import (
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/spec"
)

func Test_titleCasing(t *testing.T) {
	tests := []struct {
		input, expect string
	}{
		{input: "", expect: ""},
		{input: "a", expect: "A"},
		{input: "ab", expect: "Ab"},
		{input: "ab a", expect: "Ab a"},
	}

	for _, tt := range tests {
		if got := toTitle(tt.input); got != tt.expect {
			t.Errorf("Got '%s', Want: '%s'", got, tt.expect)
		}
	}
}

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

func Test_sseWriter_Write(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   \t\n",
			expected: "",
		},
		{
			name:     "replaces SOH separators",
			input:    "8=FIX.4.4\x019=100\x0135=D\x01",
			expected: `<div class="text-blue-700 dark:text-blue-400">&gt; 8=FIX.4.4|9=100|35=D|</div>`,
		},
		{
			name:     "no SOH",
			input:    "8=FIX.4.4|9=100|35=D",
			expected: `<div class="text-blue-700 dark:text-blue-400">&gt; 8=FIX.4.4|9=100|35=D</div>`,
		},
		{
			name:     "trims surrounding whitespace",
			input:    "  8=FIX.4.4\x019=100\x01  \n",
			expected: `<div class="text-blue-700 dark:text-blue-400">&gt; 8=FIX.4.4|9=100|</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := make(chan string, 1)
			w := &sseWriter{stream: stream}

			gotN, err := w.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else if gotN != len([]byte(tt.input)) {
				t.Errorf("Write returned %d bytes, want %d", gotN, len([]byte(tt.input)))
			}

			select {
			case got := <-stream:
				if got != tt.expected {
					t.Errorf("stream output = %q, want %q", got, tt.expected)
				}

				if strings.Contains(got, "\x01") {
					t.Errorf("stream output still contains SOH character: %q", got)
				}

			default:
				if tt.expected != "" {
					t.Fatal("expected output on stream")
				}
			}
		})
	}
}
