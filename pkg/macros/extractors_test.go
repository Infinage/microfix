package macros

import (
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

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
			name:          "Incoming Explicit Third Instance (Fails)",
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
