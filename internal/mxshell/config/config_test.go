package config

import (
	"path/filepath"
	"testing"
)

func TestConfig_LoadAndDump(t *testing.T) {
	// Create an isolated temp directory for the test
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, ".mxrc")

	// Create a dummy config and dump it
	original := &Config{
		SenderCompID: "TEST_SENDER",
		TargetCompID: "TEST_TARGET",
		HeartbeatInt: 45,
		IpAddr:       "127.0.0.1",
		Port:         9999,
	}

	err := DumpConfig(tempFile, original)
	if err != nil {
		t.Fatalf("Failed to dump config: %v", err)
	}

	// Load it back and verify
	loaded, err := LoadConfig(tempFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loaded.SenderCompID != original.SenderCompID {
		t.Errorf("Expected SenderCompID %s, got %s", original.SenderCompID, loaded.SenderCompID)
	}
	if loaded.Port != original.Port {
		t.Errorf("Expected Port %d, got %d", original.Port, loaded.Port)
	}
}

func TestConfig_MissingFileReturnsError(t *testing.T) {
	_, err := LoadConfig("/path/that/does/not/exist.json")
	if err == nil {
		t.Error("Expected an error when loading a non-existent file, got nil")
	}
}
