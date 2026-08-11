package migrate

import (
	"strings"
	"testing"
)

func TestExtractAliasFromMiniFIX_Happy(t *testing.T) {
	xmlData := `<Config class_id="0" tracking_level="0" version="0">
    <baseConf class_id="1" tracking_level="0" version="0">
        <Software_MiniFIX class_id="2" tracking_level="0" version="0">
            <count>14</count>
        </Software_MiniFIX>
    </baseConf>
    <batchConf>
        <Software_MiniFIX_Batch>
            <count>2</count>
        </Software_MiniFIX_Batch>
    </batchConf>
    <transConf>
        <Software_MiniFIX_Transaction>
            <count>4</count>
            <item_version>0</item_version>
            <item>
                <first>Advertisement</first>
                <second>0 0 6 0 5 +35=7 3 +2= 3 +5= 4 +55= 3 +4= 4 +53=</second>
            </item>
            <item>
                <first>Logon</first>
                <second>0 0 3 0 5 +35=A 5 +98=0 11 +108=$HBINT</second>
            </item>
            <item>
                <first>Logout</first>
                <second>0 0 1 0 5 +35=5</second>
            </item>
            <item>
                <first>NewOrderSingle</first>
                <second>0 0 9 0 5 +35=D 11 +11=$UNIQUE 5 +21=1 12 +55=ERICB.ST 5 +54=1 14 +60=$TIMESTAMP 5 +40=2 6 +44=50 8 +38=1000</second>
            </item>
        </Software_MiniFIX_Transaction>
    </transConf>
</Config>`

	aliases, err := ExtractAliasFromMiniFIX(strings.NewReader(xmlData))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Table-driven test to spot check the parser logic
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "Advertisement",
			expected: "35=7|2=|5=|55=|4=|53=|",
		},
		{
			name:     "Logon",
			expected: "35=A|98=0|108=$HBINT|",
		},
		{
			name:     "Logout",
			expected: "35=5|",
		},
		{
			name:     "NewOrderSingle",
			expected: "35=D|11=$UNIQUE|21=1|55=ERICB.ST|54=1|60=$TIMESTAMP|40=2|44=50|38=1000|",
		},
	}

	for _, tc := range tests {
		actual, ok := aliases[tc.name]
		if !ok {
			t.Errorf("missing expected alias in map: %q", tc.name)
			continue
		}

		if actual != tc.expected {
			t.Errorf("alias %q:\nexpected: %q\ngot:      %q", tc.name, tc.expected, actual)
		}
	}
}

func TestExtractAliasFromMiniFIX_Unhappy(t *testing.T) {
	tests := []struct {
		name        string
		xmlData     string
		expectError bool
		errContains string
	}{
		{
			name: "Count Mismatch",
			xmlData: `<Config><transConf><Software_MiniFIX_Transaction>
				<count>5</count>
				<item><first>A</first><second>0 0 0 0</second></item>
			</Software_MiniFIX_Transaction></transConf></Config>`,
			expectError: true,
			errContains: "expected 5 items, found 1",
		},
		{
			name:        "Malformed XML",
			xmlData:     `<Config><transConf><Software_MiniFIX_Transaction><count>UNCLOSED TAGS`,
			expectError: true,
			errContains: "minifix xml parse failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ExtractAliasFromMiniFIX(strings.NewReader(tc.xmlData))
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected an error, but got none")
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error to contain %q, got: %v", tc.errContains, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseMiniFIXTransaction_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		rawInput    string
		expectError bool
		errContains string
	}{
		{
			name:        "Empty String",
			rawInput:    "",
			expectError: true,
			errContains: "expected: `%d %d %d %d ...`",
		},
		{
			name:        "Insufficient Header Prefix",
			rawInput:    "0 0 1",
			expectError: true,
			errContains: "expected: `%d %d %d %d ...`",
		},
		{
			name:        "Missing Space After Size",
			rawInput:    "0 0 1 0 5+35=A",
			expectError: true,
			errContains: "expected space after size",
		},
		{
			name:        "Invalid Marker",
			rawInput:    "0 0 1 0 5 *35=A",
			expectError: true,
			errContains: "unknown marker *",
		},
		{
			name:        "Missing Equals Separator",
			rawInput:    "0 0 1 0 4 +35A",
			expectError: true,
			errContains: "missing '=' seperator",
		},
		{
			name:        "Invalid Tag Type (Not a Number)",
			rawInput:    "0 0 1 0 5 +XX=A",
			expectError: true,
			errContains: "invalid tag",
		},
		{
			name:        "Size Exceeds Remaining Buffer",
			rawInput:    "0 0 1 0 999 +35=A",
			expectError: true,
			errContains: "expected 999 bytes",
		},
		{
			name:        "Zero Size Field (Should Skip)",
			rawInput:    "0 0 2 0 0 5 +35=A",
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMiniFIXTransaction(tc.rawInput)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error containing %q, got none", tc.errContains)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error to contain %q, got: %v", tc.errContains, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
