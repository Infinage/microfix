package macros

import (
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

func TestExtractMessageMarkers(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		expected  messageMarkers
		expectErr bool
	}{
		{
			name:      "Tag Only",
			args:      []string{"35"},
			expected:  messageMarkers{tag: 35, instance: 1, start: 0, end: -1},
			expectErr: false,
		},
		{
			name:      "Tag and Instance",
			args:      []string{"55", "2"},
			expected:  messageMarkers{tag: 55, instance: 2, start: 0, end: -1},
			expectErr: false,
		},
		{
			name:      "Tag, Instance, and End",
			args:      []string{"52", "1", "8"},
			expected:  messageMarkers{tag: 52, instance: 1, start: 0, end: 8},
			expectErr: false,
		},
		{
			name:      "Tag, Instance, Start, and End",
			args:      []string{"52", "1", "9", "21"},
			expected:  messageMarkers{tag: 52, instance: 1, start: 9, end: 21},
			expectErr: false,
		},
		{
			name:      "Invalid Tag",
			args:      []string{"ABC"},
			expectErr: true,
		},
		{
			name:      "Invalid Instance (Zero)",
			args:      []string{"35", "0"},
			expectErr: true,
		},
		{
			name:      "Invalid Start Index",
			args:      []string{"52", "1", "-5", "10"},
			expectErr: true,
		},
		{
			name:      "Invalid End Index",
			args:      []string{"52", "1", "0", "bad"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := extractMessageMarkers(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("Expected an error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if res != tt.expected {
				t.Errorf("Expected %+v, got %+v", tt.expected, res)
			}
		})
	}
}

func TestMessageMarkersExtract(t *testing.T) {
	raw := "8=FIX.4.4|35=D|52=20260404-12:00:00.000|55=AAPL|55=GOOG|10=000|"
	msg, err := message.MessageFromString(raw, "|")
	if err != nil {
		t.Fatalf("Failed to construct message: %v", err)
	}

	tests := []struct {
		name      string
		marker    messageMarkers
		expected  string
		expectErr bool
	}{
		{
			name:      "Extract Full Value (Default)",
			marker:    messageMarkers{tag: 35, instance: 1, start: 0, end: -1},
			expected:  "D",
			expectErr: false,
		},
		{
			name:      "Extract Second Instance",
			marker:    messageMarkers{tag: 55, instance: 2, start: 0, end: -1},
			expected:  "GOOG",
			expectErr: false,
		},
		{
			name:      "Tag Not Found",
			marker:    messageMarkers{tag: 999, instance: 1, start: 0, end: -1},
			expectErr: true,
		},
		{
			name:      "Instance Not Found",
			marker:    messageMarkers{tag: 55, instance: 3, start: 0, end: -1},
			expectErr: true,
		},
		{
			name:      "Slice End Only (Date part)",
			marker:    messageMarkers{tag: 52, instance: 1, start: 0, end: 8},
			expected:  "20260404",
			expectErr: false,
		},
		{
			name:      "Slice Start and End (Time part)",
			marker:    messageMarkers{tag: 52, instance: 1, start: 9, end: 21},
			expected:  "12:00:00.000",
			expectErr: false,
		},
		{
			name:      "Slice Out of Bounds (End too large)",
			marker:    messageMarkers{tag: 52, instance: 1, start: 9, end: 999},
			expected:  "12:00:00.000", // Should safely cap at string length
			expectErr: false,
		},
		{
			name:      "Slice Out of Bounds (Start too large)",
			marker:    messageMarkers{tag: 52, instance: 1, start: 999, end: 999},
			expected:  "", // Should safely return empty string
			expectErr: false,
		},
		{
			name:      "Slice Start Greater Than End",
			marker:    messageMarkers{tag: 52, instance: 1, start: 8, end: 4},
			expected:  "", // Should safely cap start to end and return empty string
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.marker.extract(msg)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("Expected error, got nil")
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
