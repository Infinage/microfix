package macros

import (
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

func TestSubstitute_Variables(t *testing.T) {
	// Initialize a dummy store for the table-driven tests
	st := store.InitStore()

	_, _, _ = st.Set("VARS.Symbol", "AAPL")
	_, _, _ = st.Set("VARS.Qty", "100")
	_, _, _ = st.Set("ALIAS.Logon", "35=A|98=0|108=30")

	msg, err := message.MessageFromString("8=FIX.4.4|35=D|", "|")
	if err != nil {
		t.Fatalf("failed to parse string as message: %v", err)
	}

	// If buffer is empty, Store.Get returns false and macro substitution fails
	if res, err := Substitute("$BUF", nil, &st, false); err == nil || res != "$BUF" {
		t.Errorf("Expected $BUF resolution to fail, but passed: %q", res)
	}

	st.SetBuffer(msg)

	tests := []struct {
		name          string
		input         string
		expected      string
		quoteIfSpaces bool
		expectErr     bool
		setup         func(*store.Store)
	}{
		{
			name:     "Standard Variables",
			input:    "8=$BUF[8]|35=D|55=$VARS.Symbol|38=$VARS.Qty|",
			expected: "8=FIX.4.4|35=D|55=AAPL|38=100|",
		},
		{
			name:     "Full buffer contents",
			input:    "$BUF",
			expected: "8=FIX.4.4|35=D|",
		},
		{
			name:      "Invalid buffer key",
			input:     "$BUF[]",
			expectErr: true,
		},
		{
			name:     "Alias Expansion",
			input:    "send $ALIAS.Logon",
			expected: "send 35=A|98=0|108=30",
		},
		{
			name:      "Missing Variable (Strict Failure)",
			input:     "35=D|55=$VARS.DoesNotExist|",
			expectErr: true,
		},
		{
			name:      "Missing Namespace (Strict Failure)",
			input:     "35=D|55=$UNKNOWN.Symbol|",
			expectErr: true,
		},
		{
			name:          "quoteIfSpaces: False with Multi-Word String",
			setup:         func(s *store.Store) { s.Set("VARS.MultiWord", "Execution Report") },
			input:         "assert $VARS.MultiWord == 'Execution Report'",
			expected:      "assert Execution Report == 'Execution Report'",
			quoteIfSpaces: false,
		},
		{
			name:          "quoteIfSpaces: True with Single-Word String",
			setup:         func(s *store.Store) { s.Set("VARS.SingleWord", "New") },
			input:         "assert $VARS.SingleWord == 'New'",
			expected:      "assert New == 'New'",
			quoteIfSpaces: true,
		},
		{
			name:          "quoteIfSpaces: True with Multi-Word String",
			setup:         func(s *store.Store) { s.Set("VARS.MultiWord", "Execution Report") },
			input:         "assert $VARS.MultiWord == 'Execution Report'",
			expected:      `assert "Execution Report" == 'Execution Report'`,
			quoteIfSpaces: true,
		},
		{
			name:          "quoteIfSpaces: True with Multi-Word String and Internal Quotes",
			setup:         func(s *store.Store) { s.Set("VARS.QuotedString", `Execution "Filled" Report`) },
			input:         "print $VARS.QuotedString",
			expected:      `print "Execution ""Filled"" Report"`,
			quoteIfSpaces: true,
		},
		{
			name: "Recursive Substitution (Valid)",
			setup: func(s *store.Store) {
				s.Set("ALIAS.Level1", "$ALIAS.Level2")
				s.Set("ALIAS.Level2", "$VARS.FinalValue")
				s.Set("VARS.FinalValue", "35=D|11=ABC|")
			},
			input:    "send $ALIAS.Level1",
			expected: "send 35=D|11=ABC|",
		},
		{
			name: "Recursive Substitution Quoted (Valid)",
			setup: func(s *store.Store) {
				s.Set("VARS.ID", "RJ-NJ")
				s.Set("ALIAS.Level1", "$ALIAS.Level2")
				s.Set("ALIAS.Level2", "$VARS.FinalValue")
				s.Set("VARS.FinalValue", "35=1|112=$VARS.ID|")
			},
			input:    `send "$ALIAS.Level1"`,
			expected: `send "35=1|112=RJ-NJ|"`,
		},
		{
			name: "Sibling Substitution (Valid, Not a Cycle)",
			setup: func(s *store.Store) {
				s.Set("VARS.ID", "ORD123")
				s.Set("ALIAS.DoubleID", "11=$VARS.ID|41=$VARS.ID|")
			},
			input:    "send $ALIAS.DoubleID",
			expected: "send 11=ORD123|41=ORD123|",
		},
		{
			name: "Circular Reference (Direct)",
			setup: func(s *store.Store) {
				s.Set("ALIAS.Loop", "$ALIAS.Loop")
			},
			input:     "send $ALIAS.Loop",
			expectErr: true,
		},
		{
			name: "Circular Reference (Indirect A->B->C->A)",
			setup: func(s *store.Store) {
				s.Set("ALIAS.A", "35=D|$ALIAS.B")
				s.Set("ALIAS.B", "41=XYZ|$ALIAS.C")
				s.Set("ALIAS.C", "11=ABC|$ALIAS.A")
			},
			input:     "send $ALIAS.A",
			expectErr: true,
		},
		{
			name: "Cross-Namespace Circular Reference",
			setup: func(s *store.Store) {
				s.Set("ALIAS.Start", "$VARS.Middle")
				s.Set("VARS.Middle", "$VARS.End")
				s.Set("VARS.End", "$ALIAS.Start")
			},
			input:     "send $ALIAS.Start",
			expectErr: true,
		},
		{
			name:     "Buffer Slicing (End only)",
			input:    "35=$BUF[35,1,1]",
			expected: "35=D",
		},
		{
			name:     "Buffer Slicing (Start and End)",
			input:    "Version=$BUF[8,1,4,7]",
			expected: "Version=4.4",
		},
	}

	// Run table-driven tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(&st)
			}

			res, err := Substitute(tt.input, nil, &st, tt.quoteIfSpaces)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("Expected an error but got nil")
				}
				return // Test passes if error is expected and caught
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if res != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, res)
			}
		})
	}

	t.Run("Recursive substitution doesn't mask errors", func(t *testing.T) {
		st.Set("VARS.A", "$VARS.B")
		st.Set("VARS.B", "$VARS.DoesNotExist")
		st.Set("ALIAS.Test", "$VARS.A|$VARS.A")
		if _, err := Substitute("print $ALIAS.Test", nil, &st, false); err == nil {
			t.Error("Expected substitution to fail, but passed")
		} else if errMsg := err.Error(); !strings.Contains(errMsg, `variable resolution failed for "$VARS.DoesNotExist"`) {
			t.Errorf("Expected a 'variable resolution failed error', got: %s", errMsg)
		}
	})

	t.Run("Magics: Unique and Timestamp", func(t *testing.T) {
		input := "11=$UNIQUE|52=$TIMESTAMP|"
		res, err := Substitute(input, nil, &st, false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if strings.Contains(res, "$UNIQUE") || strings.Contains(res, "$TIMESTAMP") {
			t.Errorf("Variables were not fully expanded: %s", res)
		}
	})

	t.Run("Snapshot Variables", func(t *testing.T) {
		sess, err := session.NewSession("FIX44.xml", "SENDER", "TARGET", 30, session.EngineOptions{})
		if err != nil {
			t.Fatalf("Failed to initialize session for test: %v", err)
		}

		input := "Status: $STATUS | In: $SEQ_IN | Out: $SEQ_OUT"
		res, err := Substitute(input, sess, &st, false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// A fresh session should evaluate to state "New" and sequence numbers
		if res != "Status: New | In: 0 | Out: 0" {
			t.Errorf("Expected snapshot to resolve to 'New', got: %s", res)
		}
	})
}

func TestSubstituteCaseInsensitive(t *testing.T) {
	st := store.InitStore()
	st.Set("VARS.Symbol", "AAPL")
	st.Set("ALIAS.ping", "35=0")

	// Set up a mock session for STATUS macro
	sess, err := session.NewSession("FIX44.xml", "SENDER", "TARGET", 30, session.EngineOptions{})
	if err != nil {
		t.Fatalf("Failed to initialize session for test: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Lowercase prefix with Store lookup",
			input:    "$vars.Symbol",
			expected: "AAPL",
		},
		{
			name:     "Mixed case prefix with Store lookup",
			input:    "$vArS.Symbol",
			expected: "AAPL",
		},
		{
			name:     "Lowercase prefix with Alias lookup",
			input:    "$alias.ping",
			expected: "35=0",
		},
		{
			name:     "Lowercase system macro",
			input:    "$status",
			expected: "New",
		},
		{
			name:     "Mixed case system macro",
			input:    "$SeQ_iN",
			expected: "0",
		},
		{
			name:     "Ensure brackets contents are NOT upper-cased",
			input:    "$lastIn[d,11]",
			expected: "no incoming message of type 'd' found in session history",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Substitute(tt.input, sess, &st, false)
			if err != nil && strings.Contains(err.Error(), tt.expected) {
				return
			} else if err != nil {
				t.Fatalf("Unexpected error during case-insensitive evaluation: %v", err)
			} else if res != tt.expected {
				t.Errorf("Expected case-insensitive resolution of %q to be %q, got %q", tt.input, tt.expected, res)
			}
		})
	}
}

func TestSubstitute_AliasParent_TwoChildrenWithCommaParams(t *testing.T) {
	st := store.InitStore()

	st.Set("ALIAS.Child1", "1=ABC\\,DEF|")
	st.Set("ALIAS.Child2", "1=GHI\\,JKL|")

	// Parent alias built from its two children
	st.Set("ALIAS.Parent", "$ALIAS.Child1$ALIAS.Child2")

	input := "send $ALIAS.Parent[1.1=GHI\\,JKL,1.2=ABC\\,DEF]"
	expected := "send 1=GHI,JKL|1=ABC,DEF|"

	res, err := Substitute(input, nil, &st, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	} else if res != expected {
		t.Errorf("Expected %q, got %q", expected, res)
	}
}

func TestSubstitute_Alias_WithBracketedMacros(t *testing.T) {
	st := store.InitStore()
	msg, err := message.MessageFromString("52=MicroFIX is awesome!|", "|")
	if err != nil {
		t.Fatal("Failed to parse message")
	}

	st.Set("ALIAS.TR", "35=1|112=TestRequestID|")
	st.SetBuffer(msg)

	input := "send $alias.TR[112=$BUF[52,1,8]]"
	want := "send 35=1|112=MicroFIX|"

	if got, err := Substitute(input, nil, &st, false); err != nil {
		t.Errorf("Unexpected error in substitution: %v", err)
	} else if got != want {
		t.Errorf("Expected to resolve as %q, got %q", got, want)
	}
}
