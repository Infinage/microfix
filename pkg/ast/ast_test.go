package ast

import (
	"reflect"
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple AND",
			input:    "35=D & 11=ABC",
			expected: []string{"35=D", "&", "11=ABC"},
		},
		{
			name:     "Complex Grouping with NOT",
			input:    "35=D & (11=ORD1 | 11=ORD2) & !39=4",
			expected: []string{"35=D", "&", "(", "11=ORD1", "|", "11=ORD2", ")", "&", "!", "39=4"},
		},
		{
			name:     "No Spaces",
			input:    "!(35=D|35=G)&11=A",
			expected: []string{"!", "(", "35=D", "|", "35=G", ")", "&", "11=A"},
		},
		{
			name:     "Excessive Spaces",
			input:    "  35=D   &    !  11=A  ",
			expected: []string{"35=D", "&", "!", "11=A"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("tokenize(%q)\n got: %v\nwant: %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewMatcher_Errors(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		errPrefix string
	}{
		{"Empty Input", []string{}, "no arguments"},
		{"Missing Closing Paren", []string{"(35=D", "|", "11=A"}, "missing closing parenthesis"},
		{"Unexpected End", []string{"35=D", "&"}, "unexpected end of expression"},
		{"Invalid Tag Format", []string{"35-D"}, "invalid operand format"},
		{"Invalid Tag Number", []string{"ABC=D"}, "invalid FIX tag"},
		{"Hanging Operator", []string{"35=D", "!", "11=A"}, "unexpected token at end of expression"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMatcher(tt.args)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errPrefix)
			}
			if !strings.Contains(err.Error(), tt.errPrefix) {
				t.Errorf("expected error to contain %q, got: %v", tt.errPrefix, err)
			}
		})
	}
}

func TestAST_Match(t *testing.T) {
	// Helper to quickly create a mocked FIX message
	makeMsg := func(raw string) *message.Message {
		// Assuming your message package uses | as a delimiter in testing
		msg, err := message.MessageFromString(raw, "|")
		if err != nil {
			t.Fatalf("Failed to create mock message: %v", err)
		}
		return &msg
	}

	tests := []struct {
		name       string
		expression string
		msgRaw     string
		want       bool
	}{
		{
			name:       "Single Tag Match",
			expression: "35=D",
			msgRaw:     "35=D|11=A|",
			want:       true,
		},
		{
			name:       "Single Tag Mismatch",
			expression: "35=D",
			msgRaw:     "35=8|11=A|",
			want:       false,
		},
		{
			name:       "Missing Tag evaluates to False",
			expression: "39=4",
			msgRaw:     "35=D|11=A|", // Tag 39 is missing entirely
			want:       false,
		},
		{
			name:       "AND Condition - True",
			expression: "35=D & 11=A",
			msgRaw:     "35=D|11=A|",
			want:       true,
		},
		{
			name:       "AND Condition - False",
			expression: "35=D & 11=A",
			msgRaw:     "35=D|11=B|",
			want:       false,
		},
		{
			name:       "OR Condition - True",
			expression: "35=D | 35=G",
			msgRaw:     "35=G|",
			want:       true,
		},
		{
			name:       "NOT Condition",
			expression: "!39=4",
			msgRaw:     "35=D|39=0|",
			want:       true,
		},
		{
			name:       "Complex Grouping: True",
			expression: "35=D & (11=ORD1 | 11=ORD2) & !39=4",
			msgRaw:     "35=D|11=ORD2|39=0|",
			want:       true,
		},
		{
			name:       "Complex Grouping: False (Fails NOT condition)",
			expression: "35=D & (11=ORD1 | 11=ORD2) & !39=4",
			msgRaw:     "35=D|11=ORD1|39=4|",
			want:       false,
		},
		{
			name:       "Precedence Test without Parens (NOT binds tightest, then AND, then OR)",
			expression: "35=D & 11=A | 35=G",
			// Evaluates as: (35=D AND 11=A) OR 35=G
			msgRaw: "35=G|11=B|", // Fails the AND, but passes the OR
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate how the executor will pass args
			args := strings.Split(tt.expression, " ")

			matcher, err := NewMatcher(args)
			if err != nil {
				t.Fatalf("Failed to compile AST: %v", err)
			}

			msg := makeMsg(tt.msgRaw)
			got := matcher.Match(msg)

			if got != tt.want {
				t.Errorf("Match() = %v, want %v\nExpr: %s\nMsg:  %s", got, tt.want, tt.expression, tt.msgRaw)
			}
		})
	}
}
