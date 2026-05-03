package config

import (
	"path/filepath"
	"testing"
)

func TestAlias_LoadAndDump(t *testing.T) {
	// Create an isolated temp directory for the test
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, ".mxaliases")

	// Create a dummy alias map and dump it
	original := Alias{
		"Logon":     "35=A|98=0|108=30|",
		"Heartbeat": "35=0|",
	}

	err := original.Dump(tempFile)
	if err != nil {
		t.Fatalf("Failed to dump aliases: %v", err)
	}

	// Load it back and verify
	loaded, err := LoadAlias(tempFile)
	if err != nil {
		t.Fatalf("Failed to load aliases: %v", err)
	}

	// Validate contents
	if (*loaded)["Logon"] != original["Logon"] {
		t.Errorf("Expected Logon alias %s, got %s", original["Logon"], (*loaded)["Logon"])
	}
	if len(*loaded) != len(original) {
		t.Errorf("Expected %d aliases, got %d", len(original), len(*loaded))
	}
}

func TestAlias_MissingFileReturnsError(t *testing.T) {
	_, err := LoadAlias("/path/that/does/not/exist.json")
	if err == nil {
		t.Error("Expected an error when loading a non-existent file, got nil")
	}
}
