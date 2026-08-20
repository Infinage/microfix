package spec

import (
	"slices"
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

func Test_RouterLegacyCheck(t *testing.T) {
	t.Run("FIX4X - LegacyRouter", func(t *testing.T) {
		router, err := NewDefaultRouter("FIX40.xml")
		if err != nil {
			t.Fatal("Failed to load router setup")
		} else if !router.IsLegacyRouter() {
			t.Error("Expected IsLegacyRouter() to return true")
		}
	})

	t.Run("FIXT - LegacyRouter", func(t *testing.T) {
		router, err := NewRouter("FIXT11.xml", []string{"FIXT11.xml"})
		if err != nil {
			t.Fatal("Failed to load router setup")
		} else if !router.IsLegacyRouter() {
			t.Error("Expected IsLegacyRouter() to return true")
		}
	})

	t.Run("FIXT - Non LegacyRouter", func(t *testing.T) {
		router, err := NewRouter("FIXT11.xml", []string{"FIX40.xml"})
		if err != nil {
			t.Fatal("Failed to load router setup")
		} else if router.IsLegacyRouter() {
			t.Error("Expected IsLegacyRouter() to return false")
		}
	})
}

func TestRouter_Salvage(t *testing.T) {
	router, err := NewDefaultRouter("FIX40")
	if err != nil {
		t.Fatalf("Failed to load router setup: %v", err)
	}

	expectedBeginString := router.SessionSpec().BeginString()

	tests := []struct {
		name     string
		input    string
		check    []uint16
		contains []string
	}{
		{
			name:  "Empty Message",
			input: "",
			check: []uint16{8, 9, 35, 49, 56, 34, 52, 10},
			contains: []string{
				"8=" + expectedBeginString,
				"35=0",
				"49=FROM",
				"56=TO",
				"34=1",
			},
		},
		{
			name:  "Partial Message Overwrites 8 and Keeps 35",
			input: "8=FIX.4.2|35=D|11=TEST|", // Wrong begin string, custom MsgType
			check: []uint16{8, 9, 35, 49, 56, 34, 52, 10, 11},
			contains: []string{
				"8=" + expectedBeginString,
				"35=D",
				"11=TEST",
				"49=FROM",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, _ := message.MessageFromString(tc.input, "|")

			salvagedMsg := router.Salvage(msg)
			outputStr := salvagedMsg.String("|")

			// Validate all critical tags exist
			for _, tag := range tc.check {
				if _, idx := salvagedMsg.FindFrom(tag, 0); idx == -1 {
					t.Errorf("Salvaged message missing expected tag %d. Result: %s", tag, outputStr)
				}
			}

			// Validate expected values (overwrites and keepers)
			for _, expectedVal := range tc.contains {
				if !strings.Contains(outputStr, expectedVal) {
					t.Errorf("Expected salvaged message to contain %q, but it didn't. Result: %s", expectedVal, outputStr)
				}
			}
		})
	}
}

func TestRouter_SalvageOrdering(t *testing.T) {
	ro, err := NewDefaultRouter("FIX40")
	if err != nil {
		t.Fatalf("Failed to load router setup: %v", err)
	}

	orderTests := []struct {
		name          string
		input         string
		expectedOrder []uint16
	}{
		{
			name:          "Body Tag precedes header",
			input:         "11=TEST|8=FIX.4.4|35=D|",
			expectedOrder: []uint16{11, 8, 9, 35, 49, 56, 34, 52, 10},
		},
		{
			name:          "Scrambled header tags",
			input:         "35=D|8=FIX.4.2|56=TO|",
			expectedOrder: []uint16{35, 8, 9, 56, 49, 34, 52, 10},
		},
		{
			name:          "Single tag",
			input:         "11=MICROFIX|",
			expectedOrder: []uint16{8, 9, 35, 49, 56, 34, 52, 11, 10},
		},
	}

	for _, tc := range orderTests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := message.MessageFromString(tc.input, "|")
			if err != nil {
				t.Errorf("Unexpected error parsing %q: %v", tc.input, err)
			}

			salvagedMsg := ro.Salvage(msg)

			// Extract just the tags to compare the ordering
			var actualOrder []uint16
			for _, field := range salvagedMsg {
				actualOrder = append(actualOrder, field.Tag)
			}
			if !slices.Equal(actualOrder, tc.expectedOrder) {
				t.Errorf("Expected %v, got %v", tc.expectedOrder, actualOrder)
			}
		})
	}
}
