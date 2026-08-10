package macros

import (
	"strings"
	"testing"
)

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
