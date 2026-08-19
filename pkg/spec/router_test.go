package spec

import (
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
