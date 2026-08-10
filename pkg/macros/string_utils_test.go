package macros

import (
	"slices"
	"testing"
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
