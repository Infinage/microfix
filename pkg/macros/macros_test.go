package macros

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

func TestSplitStoreKey(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		key    string
		params string
	}{
		{
			name:   "No params",
			raw:    "$alias.TR",
			key:    "alias.TR",
			params: "",
		},
		{
			name:   "Empty params (Valid)",
			raw:    "$ALIAS.TR[]",
			key:    "ALIAS.TR",
			params: "",
		},
		{
			name:   "Valid params",
			raw:    "$ALIAS.TR[...]",
			key:    "ALIAS.TR",
			params: "...",
		},
		{
			name:   "Valid params with brackets",
			raw:    "$ALIAS.TR[[[]]",
			key:    "ALIAS.TR",
			params: "[[]",
		},
		{
			name:   "Valid real world scenario",
			raw:    "$alias.TR[112.1=$vars.ReqID]  ",
			key:    "alias.TR",
			params: "112.1=$vars.ReqID",
		},
		{
			name:   "Invalid params return as key",
			raw:    "$Alias.TR[ ",
			key:    "Alias.TR[",
			params: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, params := splitStoreKey(tt.raw)
			if key != tt.key || params != tt.params {
				t.Errorf("Expected (%s, %s), got (%s, %s)", key, params, tt.key, tt.params)
			}
		})
	}
}

func TestExtractSBrackets(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		expected  []string
		expectErr bool
	}{
		{"Valid Positive", "$DATE[+3]", []string{"+3"}, false},
		{"Valid Negative", "$DATE[-5]", []string{"-5"}, false},
		{"Valid CSV", "$LASTIN[D, 11]", []string{"D", "11"}, false},
		{"Valid Empty", "$DATE[]", nil, false},
		{"Missing Brackets", "$DATE", nil, true},
		{"Missing Close Bracket", "$DATE[+3", nil, true},
		{"Missing Open Bracket", "$DATE+3]", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := extractSBrackets(tt.raw)
			if (err != nil) != tt.expectErr {
				t.Errorf("extractSBrackets(%q) error = %v, expectErr %v", tt.raw, err, tt.expectErr)
				return
			}
			if !slices.Equal(res, tt.expected) {
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

func TestSubstituteMessageTag(t *testing.T) {
	// Mock function that always return empty
	emptyMockFn := func(msgType string, isIncoming bool) *message.Message {
		return nil
	}

	raw := "8=FIX.4.4|9=120|35=V|49=SENDER|56=TARGET|34=1|52=20260404-12:00:00.000|262=REQ1|146=2|55=AAPL|55=GOOG|10=000|"
	msg, err := message.MessageFromString(raw, "|")
	if err != nil {
		t.Fatalf("Failed to construct message: %v", err)
	}

	// Mock function that returns our dummy message only for MsgType 'V'
	mockMsgFn := func(msgType string, isIncoming bool) *message.Message {
		if msgType == "V" {
			return &msg
		}
		return nil
	}

	tests := []struct {
		name          string
		input         string
		expectErr     bool
		errString     string
		resString     string
		mockLastMsgFn func(string, bool) *message.Message
	}{
		{
			name:          "Invalid Syntax - Too few arguments",
			input:         "$LASTIN[D]",
			expectErr:     true,
			errString:     "invalid syntax \"$LASTIN[D]\": expected $[MsgType,Tag[,Instance[,End]|,Start,End]]",
			mockLastMsgFn: emptyMockFn,
		},
		{
			name:          "Invalid Syntax - Too many arguments",
			input:         "$LASTIN[D,11,2,5,0,0]",
			expectErr:     true,
			errString:     "invalid syntax \"$LASTIN[D,11,2,5,0,0]\": expected $[MsgType,Tag",
			mockLastMsgFn: emptyMockFn,
		},
		{
			name:          "Invalid Tag - Not a number",
			input:         "$LASTIN[D,abc]",
			expectErr:     true,
			errString:     `invalid tag "abc"`,
			mockLastMsgFn: emptyMockFn,
		},
		{
			name:          "Invalid Count - Not a number",
			input:         "$LASTOUT[8,11,xyz]",
			expectErr:     true,
			errString:     "invalid instance count \"xyz\" (expected > 0)",
			mockLastMsgFn: emptyMockFn,
		},
		{
			name:          "Invalid Count - Zero",
			input:         "$LASTIN[D,11,0]",
			expectErr:     true,
			errString:     "invalid instance count",
			mockLastMsgFn: emptyMockFn,
		},
		{
			name:          "Valid Syntax - Fails at Session Lookup",
			input:         "$LASTIN[D,11,2]",
			expectErr:     true,
			errString:     "no incoming message of type 'D' found in session history",
			mockLastMsgFn: emptyMockFn,
		},
		{
			name:          "Incoming Default Instance",
			input:         "$LASTIN[V,55]",
			resString:     "AAPL",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Incoming Explicit First Instance",
			input:         "$LASTIN[V,55,1]",
			resString:     "AAPL",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Incoming Explicit Second Instance",
			input:         "$LASTIN[V,55,2]",
			resString:     "GOOG",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Incoming Explicit Second Instance",
			input:         "$LASTIN[V,55,3]",
			expectErr:     true,
			errString:     "tag 55 (instance 3) not found",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Outgoing Tag",
			input:         "$LASTOUT[V,146]",
			resString:     "2",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Incoming Explicit Second Instance",
			input:         "$LASTIN[V,55,3]",
			expectErr:     true,
			errString:     "tag 55 (instance 3) not found",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Outgoing Tag",
			input:         "$LASTOUT[V,146]",
			resString:     "2",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Slice End Only (Extract Date from Tag 52)",
			input:         "$LASTIN[V,52,1,8]",
			resString:     "20260404",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Slice Start and End (Extract Time from Tag 52)",
			input:         "$LASTIN[V,52,1,9,21]",
			resString:     "12:00:00.000",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Slice Out of Bounds (End too large - caps safely)",
			input:         "$LASTIN[V,52,1,999]",
			resString:     "20260404-12:00:00.000",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Slice Out of Bounds (Start too large - returns empty)",
			input:         "$LASTIN[V,52,1,999,999]",
			resString:     "",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Slice Start Greater Than End (Safely returns empty)",
			input:         "$LASTIN[V,52,1,8,4]",
			resString:     "",
			mockLastMsgFn: mockMsgFn,
		},
		{
			name:          "Invalid Slice Arguments (Start not an int)",
			input:         "$LASTIN[V,52,1,abc,8]",
			expectErr:     true,
			errString:     `invalid start index "abc"`,
			mockLastMsgFn: mockMsgFn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := substituteMessageTag(tt.input, strings.HasPrefix(tt.input, "$LASTIN"), tt.mockLastMsgFn)
			if tt.expectErr && err == nil {
				t.Fatalf("Expected an error but got nil for input: %s", tt.input)
			} else if !tt.expectErr && err != nil {
				t.Fatalf("Unexpected error for input %q, %s", tt.input, err)
			}

			if tt.expectErr && !strings.Contains(err.Error(), tt.errString) {
				t.Errorf("Expected error to contain %q, but got %q", tt.errString, err.Error())
			} else if !tt.expectErr && !strings.Contains(res, tt.resString) {
				t.Errorf("Expected %s, got %s", tt.resString, res)
			}
		})
	}
}

func TestSubstituteRandom(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		expectedLen int
		expectErr   bool
		isUUID      bool
	}{
		{"Default UUID", "$UNIQUE", 36, false, true},
		{"Valid Custom Length", "$UNIQUE[15]", 15, false, false},
		{"Valid Max Length Capped", "$UNIQUE[2000]", 1000, false, false},
		{"Invalid Zero Length", "$UNIQUE[0]", 0, true, false},
		{"Invalid Negative Length", "$UNIQUE[-10]", 0, true, false},
		{"Invalid Format", "$UNIQUE[abc]", 0, true, false},
		{"Missing Length Parameter", "$UNIQUE[]", 0, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := substituteRandom(tt.raw)
			if tt.expectErr && err != nil {
				return
			}

			if (err != nil) != tt.expectErr {
				t.Errorf("substituteRandom(%q) error = %v, expectErr %v", tt.raw, err, tt.expectErr)
				return
			}

			if len(res) != tt.expectedLen {
				t.Errorf("Expected length %d, got %d", tt.expectedLen, len(res))
			} else if tt.isUUID {
				matched, err := regexp.MatchString(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`, res)
				if err != nil || !matched {
					t.Errorf("Expected valid UUID format, got: %s", res)
				}
			}
		})
	}

	t.Run("Randomness Check", func(t *testing.T) {
		res1, _ := substituteRandom("$UNIQUE[20]")
		res2, _ := substituteRandom("$UNIQUE[20]")
		if res1 == "" || res2 == "" {
			t.Fatal("substituteRandom returned an empty string")
		} else if res1 == res2 {
			t.Fatalf("Expected consecutive calls to generate unique strings, but got duplicates: %s", res1)
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

func TestSubstituteMessageWithParamString(t *testing.T) {
	// Repeated tag 55 (two Symbol instances) lets us exercise instance selection
	raw := "8=FIX.4.4|35=D|55=AAPL|55=GOOG|54=1|38=100|40=2|"

	tests := []struct {
		name           string
		raw            string
		paramStr       string
		unescapeCommas bool
		expected       string
		expectErr      bool
		errString      string
	}{
		{
			name:     "Single override, default instance",
			raw:      raw,
			paramStr: "54=2",
			expected: "8=FIX.4.4|35=D|55=AAPL|55=GOOG|54=2|38=100|40=2|",
		},
		{
			name:     "Explicit instance 1 matches default",
			raw:      raw,
			paramStr: "55.1=MSFT",
			expected: "8=FIX.4.4|35=D|55=MSFT|55=GOOG|54=1|38=100|40=2|",
		},
		{
			name:     "Override second instance of repeated tag",
			raw:      raw,
			paramStr: "55.2=TSLA",
			expected: "8=FIX.4.4|35=D|55=AAPL|55=TSLA|54=1|38=100|40=2|",
		},
		{
			name:     "Multiple simultaneous overrides across different tags",
			raw:      raw,
			paramStr: "54=1,38=500,55.2=NFLX",
			expected: "8=FIX.4.4|35=D|55=AAPL|55=NFLX|54=1|38=500|40=2|",
		},
		{
			name:      "Tag not present in message at all - no insertion allowed",
			raw:       raw,
			paramStr:  "999=abc",
			expectErr: true,
			errString: "tag 999 (instance 1) not found in alias payload",
		},
		{
			name:      "Instance requested exceeds existing count",
			raw:       raw,
			paramStr:  "55.3=IBM",
			expectErr: true,
			errString: "tag 55 (instance 3) not found in alias payload",
		},
		{
			name:      "Invalid tag - not numeric",
			raw:       raw,
			paramStr:  "abc=xyz",
			expectErr: true,
			errString: "invalid tag abc",
		},
		{
			name:      "Invalid instance - not numeric",
			raw:       raw,
			paramStr:  "55.abc=xyz",
			expectErr: true,
			errString: "invalid instance abc (expected >= 1)",
		},
		{
			name:      "Invalid instance - zero",
			raw:       raw,
			paramStr:  "55.0=xyz",
			expectErr: true,
			errString: "invalid instance 0 (expected >= 1)",
		},
		{
			name:      "Invalid instance - negative",
			raw:       raw,
			paramStr:  "55.-1=xyz",
			expectErr: true,
			errString: "invalid instance -1 (expected >= 1)",
		},
		{
			name:      "Missing '=value' entirely",
			raw:       raw,
			paramStr:  "112",
			expectErr: true,
			errString: "missing '=value' for parameter ending at",
		},
		{
			name:      "Dangling key after trailing comma",
			raw:       raw,
			paramStr:  "54=2,999",
			expectErr: true,
			errString: "missing '=value' for parameter ending at",
		},
		{
			name:           "Escaped comma preserved when unescapeCommas is false",
			raw:            raw,
			paramStr:       `55.2=NEW\,YORK`,
			unescapeCommas: false,
			expected:       `8=FIX.4.4|35=D|55=AAPL|55=NEW\,YORK|54=1|38=100|40=2|`,
		},
		{
			name:           "Escaped comma unescaped in final output when unescapeCommas is true",
			raw:            raw,
			paramStr:       `55.2=NEW\,YORK`,
			unescapeCommas: true,
			expected:       "8=FIX.4.4|35=D|55=AAPL|55=NEW,YORK|54=1|38=100|40=2|",
		},
		{
			name:      "Duplicate parameter keys are rejected",
			raw:       raw,
			paramStr:  "54=1,54.1=2",
			expectErr: true,
			errString: `duplicate parameter keys are not allowed: "54.1"`,
		},
		{
			name:      "Trailing dot with no instance digits is rejected",
			raw:       raw,
			paramStr:  "54.=9",
			expectErr: true,
			errString: `invalid syntax: key has a trailing '.'`,
		},
		{
			name:      "Malformed raw message propagates parse error",
			raw:       "not a valid fix message",
			paramStr:  "35=D",
			expectErr: true,
		},
		{
			name:           "Empty paramStr unescapes commas when unescapeCommas is true",
			raw:            `55=NEW\,YORK`,
			paramStr:       "",
			unescapeCommas: true,
			expected:       "55=NEW,YORK",
		},
		{
			name:     "Unrelated fields and tag order remain untouched",
			raw:      raw,
			paramStr: "38=999",
			expected: "8=FIX.4.4|35=D|55=AAPL|55=GOOG|54=1|38=999|40=2|",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := substituteMessageWithParamString(tt.raw, tt.paramStr, tt.unescapeCommas)

			if tt.expectErr {
				if err == nil {
					t.Fatalf("Expected an error but got nil (result: %q)", res)
				}
				if tt.errString != "" && !strings.Contains(err.Error(), tt.errString) {
					t.Errorf("Expected error to contain %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if res != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, res)
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
