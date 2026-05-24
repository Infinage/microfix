package executor

import (
	"regexp"
	"strings"
	"testing"
	"time"

	script "github.com/infinage/microfix/pkg/executor/internal/handlers"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

func TestUUID(t *testing.T) {
	u1 := uuid()
	u2 := uuid()

	if u1 == "" || u2 == "" {
		t.Fatal("uuid() returned an empty string")
	}

	if u1 == u2 {
		t.Fatal("uuid() generated identical strings, expected randomness")
	}

	// Verify standard 8-4-4-4-12 hex format
	matched, err := regexp.MatchString(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`, u1)
	if err != nil || !matched {
		t.Errorf("uuid() format invalid: %s", u1)
	}
}

func TestExtractSBrackets(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		expected  string
		expectErr bool
	}{
		{"Valid Positive", "$DATE[+3]", "+3", false},
		{"Valid Negative", "$DATE[-5]", "-5", false},
		{"Valid CSV", "$LASTIN[D, 11]", "D, 11", false},
		{"Valid Empty", "$DATE[]", "", false},
		{"Missing Brackets", "$DATE", "", true},
		{"Missing Close Bracket", "$DATE[+3", "", true},
		{"Missing Open Bracket", "$DATE+3]", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := extractSBrackets(tt.raw)
			if (err != nil) != tt.expectErr {
				t.Errorf("extractSBrackets(%q) error = %v, expectErr %v", tt.raw, err, tt.expectErr)
				return
			}
			if res != tt.expected {
				t.Errorf("extractSBrackets(%q) = %q, want %q", tt.raw, res, tt.expected)
			}
		})
	}
}

func TestSubstituteDate(t *testing.T) {
	today := time.Now()
	todayStr := today.Format("20060102")
	tomorrowStr := today.AddDate(0, 0, 1).Format("20060102")
	yesterdayStr := today.AddDate(0, 0, -1).Format("20060102")

	tests := []struct {
		name      string
		raw       string
		expected  string
		expectErr bool
	}{
		{"Today", "$DATE", todayStr, false},
		{"Tomorrow", "$DATE[1]", tomorrowStr, false},
		{"Explicit Positive", "$DATE[+1]", tomorrowStr, false},
		{"Yesterday", "$DATE[-1]", yesterdayStr, false},
		{"Invalid Format", "$DATE[abc]", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := substituteDate(tt.raw)
			if (err != nil) != tt.expectErr {
				t.Errorf("substituteDate(%q) error = %v, expectErr %v", tt.raw, err, tt.expectErr)
				return
			}
			if res != tt.expected {
				t.Errorf("substituteDate(%q) = %q, want %q", tt.raw, res, tt.expected)
			}
		})
	}
}

func TestSubstitute_Variables(t *testing.T) {
	// Initialize a dummy store
	st := store.InitStore()
	_, _, _ = st.Set("VARS.Symbol", "AAPL")
	_, _, _ = st.Set("VARS.Qty", "100")
	_, _, _ = st.Set("ALIAS.Logon", "35=A|98=0|108=30")

	// Set up the execution context
	ctx := &script.ScriptContext{Store: &st, Session: nil}

	t.Run("Standard Variables", func(t *testing.T) {
		input := "35=D|55=$VARS.Symbol|38=$VARS.Qty|"
		expected := "35=D|55=AAPL|38=100|"

		res, err := Substitute(input, ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res != expected {
			t.Errorf("Expected %q, got %q", expected, res)
		}
	})

	t.Run("Alias Expansion", func(t *testing.T) {
		input := "send $ALIAS.Logon"
		expected := "send 35=A|98=0|108=30"

		res, err := Substitute(input, ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if res != expected {
			t.Errorf("Expected %q, got %q", expected, res)
		}
	})

	t.Run("Magics: Unique and Timestamp", func(t *testing.T) {
		input := "11=$UNIQUE|52=$TIMESTAMP|"
		res, err := Substitute(input, ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if strings.Contains(res, "$UNIQUE") || strings.Contains(res, "$TIMESTAMP") {
			t.Errorf("Variables were not fully expanded: %s", res)
		}
	})

	t.Run("Missing Variable (Strict Failure)", func(t *testing.T) {
		input := "35=D|55=$VARS.DoesNotExist|"
		_, err := Substitute(input, ctx)
		if err == nil {
			t.Error("Expected an error for a missing variable, got nil")
		}
	})

	t.Run("Missing Namespace (Strict Failure)", func(t *testing.T) {
		input := "35=D|55=$UNKNOWN.Symbol|"
		_, err := Substitute(input, ctx)
		if err == nil {
			t.Error("Expected an error for an unknown prefix, got nil")
		}
	})

	t.Run("Snapshot Variables", func(t *testing.T) {
		sess, err := session.NewSession("FIX44.xml", "SENDER", "TARGET", 30, session.EngineOptions{})
		if err != nil {
			t.Fatalf("Failed to initialize session for test: %v", err)
		}

		// Create a separate context containing the active session
		snapCtx := &script.ScriptContext{Store: &st, Session: sess}

		input := "Status: $STATUS | In: $SEQ_IN | Out: $SEQ_OUT"
		res, err := Substitute(input, snapCtx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// A fresh session should evaluate to state "New" and sequence numbers
		if res != "Status: New | In: 0 | Out: 0" {
			t.Errorf("Expected snapshot to resolve to 'New', got: %s", res)
		}
	})
}
