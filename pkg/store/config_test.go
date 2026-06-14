package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_LoadAndDump(t *testing.T) {
	// Create an isolated temp directory for the test
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, ".mxrc")

	// Create a dummy config with aliases and dump it
	original := &Config{
		SenderCompID: "TEST_SENDER",
		TargetCompID: "TEST_TARGET",
		HeartbeatInt: 45,
		IpAddr:       "127.0.0.1",
		Port:         9999,
		Alias: map[string]string{
			"Logon":     "35=A|98=0|108=30|",
			"Heartbeat": "35=0|",
		},
	}

	err := original.dump(tempFile)
	if err != nil {
		t.Fatalf("Failed to dump config: %v", err)
	}

	// Load it back and verify
	loaded, err := loadConfig(tempFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loaded.SenderCompID != original.SenderCompID {
		t.Errorf("Expected SenderCompID %s, got %s", original.SenderCompID, loaded.SenderCompID)
	}
	if loaded.Port != original.Port {
		t.Errorf("Expected Port %d, got %d", original.Port, loaded.Port)
	}
	if loaded.Alias["Logon"] != original.Alias["Logon"] {
		t.Errorf("Expected Logon alias %s, got %s", original.Alias["Logon"], loaded.Alias["Logon"])
	}
}

func TestConfig_MissingFileReturnsError(t *testing.T) {
	_, err := loadConfig("/path/that/does/not/exist.json")
	if err == nil {
		t.Error("Expected an error when loading a non-existent file, got nil")
	}
}

func TestConfig_AliasOperations(t *testing.T) {
	cfg := &Config{}

	// Test Get on nil map
	if _, ok := cfg.getAlias("NewOrder"); ok {
		t.Error("Expected false when getting from nil Alias map")
	}

	// Test Set (Insert)
	oldVal, ok := cfg.setAlias("NewOrder", "35=D|")
	if ok || oldVal != "" {
		t.Errorf("Expected (false, empty) on insert, got (%v, %s)", ok, oldVal)
	}

	// Test Get (Exists)
	val, ok := cfg.getAlias("NewOrder")
	if !ok || val != "35=D|" {
		t.Errorf("Expected (true, '35=D|'), got (%v, %s)", ok, val)
	}

	// Test Set (Update)
	oldVal, ok = cfg.setAlias("NewOrder", "35=D|11=ABC|")
	if !ok || oldVal != "35=D|" {
		t.Errorf("Expected (true, '35=D|') on update, got (%v, %s)", ok, oldVal)
	}

	// Test Delete
	oldVal, ok = cfg.deleteAlias("NewOrder")
	if !ok || oldVal != "35=D|11=ABC|" {
		t.Errorf("Expected (true, '35=D|11=ABC|') on delete, got (%v, %s)", ok, oldVal)
	}

	// Test Delete (Non-existent)
	_, ok = cfg.deleteAlias("Missing")
	if ok {
		t.Error("Expected false when deleting non-existent alias")
	}
}

func TestConfig_StrictFieldReflection(t *testing.T) {
	cfg := &Config{
		SenderCompID:      "SENDER",
		HeartbeatInt:      30,
		Port:              1234,
		FixValidateStrict: true,
		Alias:             make(map[string]string),
	}

	t.Run("Get Valid Fields", func(t *testing.T) {
		val, err := cfg.getField("SenderCompID")
		if err != nil || val != "SENDER" {
			t.Errorf("GetField SenderCompID failed: %v, %s", err, val)
		}

		val, err = cfg.getField("Port")
		if err != nil || val != "1234" {
			t.Errorf("GetField Port failed: %v, %s", err, val)
		}

		val, err = cfg.getField("FixValidateStrict")
		if err != nil || val != "true" {
			t.Errorf("GetField FixValidateStrict failed: %v, %s", err, val)
		}
	})

	t.Run("Set Valid Fields", func(t *testing.T) {
		oldVal, err := cfg.setField("HeartbeatInt", "60")
		if err != nil || oldVal != "30" {
			t.Errorf("SetField HeartbeatInt failed: %v, old: %s", err, oldVal)
		}
		if cfg.HeartbeatInt != 60 {
			t.Errorf("HeartbeatInt not updated properly, got: %d", cfg.HeartbeatInt)
		}

		oldVal, err = cfg.setField("FixValidateStrict", "false")
		if err != nil || oldVal != "true" {
			t.Errorf("SetField FixValidateStrict (false) failed: %v, old: %s", err, oldVal)
		}
		if cfg.FixValidateStrict != false {
			t.Errorf("FixValidateStrict not updated properly, got: %v", cfg.FixValidateStrict)
		}

		oldVal, err = cfg.setField("FixValidateStrict", "")
		if err != nil || oldVal != "false" {
			t.Errorf("SetField FixValidateStrict (empty) failed: %v, old: %s", err, oldVal)
		}
		if cfg.FixValidateStrict != false {
			t.Errorf("FixValidateStrict not updated properly, got: %v", cfg.FixValidateStrict)
		}
	})

	t.Run("Set Invalid Types", func(t *testing.T) {
		_, err := cfg.setField("Port", "not-a-number")
		if err == nil {
			t.Error("Expected error when setting Port to string")
		}

		_, err = cfg.setField("FixValidateStrict", "yes")
		if err == nil {
			t.Error("Expected error when setting Bool to invalid string")
		}
	})

	t.Run("Access Invalid Fields", func(t *testing.T) {
		_, err := cfg.getField("MissingField")
		if err == nil {
			t.Error("Expected error when getting MissingField")
		}

		_, err = cfg.setField("MissingField", "value")
		if err == nil {
			t.Error("Expected error when setting MissingField")
		}
	})

	t.Run("Protect Alias Field", func(t *testing.T) {
		_, err := cfg.getField("Alias")
		if err == nil {
			t.Error("Expected error when trying to GetField('Alias')")
		}

		_, err = cfg.setField("Alias", "broken")
		if err == nil {
			t.Error("Expected error when trying to SetField('Alias')")
		}
	})
}

func TestConfig_InitConfig(t *testing.T) {
	// IMPORTANT: Because os.Chdir changes the directory for the entire process,
	// do NOT use t.Parallel() in this test suite.

	// Save the original working directory so we can restore it after tests
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get original working directory: %v", err)
	}
	defer os.Chdir(originalWD)

	t.Run("Creates new config in CWD when none exist", func(t *testing.T) {
		cwdDir := t.TempDir()
		homeDir := t.TempDir()

		// Mock the environment
		os.Chdir(cwdDir)
		t.Setenv("HOME", homeDir)        // Mock Home for Unix/Linux/Mac
		t.Setenv("USERPROFILE", homeDir) // Mock Home for Windows

		// Execute
		expectedPath := filepath.Join(cwdDir, ".mxrc")
		if cfg, loadedPath := initConfig(); loadedPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, loadedPath)
		} else if cfg.SenderCompID != "SENDER" {
			t.Errorf("Expected default SenderCompID 'SENDER', got %s", cfg.SenderCompID)
		}

		// Verify that file was not actually written, requires a manual save
		if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
			t.Error("Expected .mxrc to be absent in CWD, but it was present")
		}
	})

	t.Run("Loads from CWD if it exists", func(t *testing.T) {
		cwdDir := t.TempDir()
		homeDir := t.TempDir()

		os.Chdir(cwdDir)
		t.Setenv("HOME", homeDir)
		t.Setenv("USERPROFILE", homeDir)

		// Create a dummy config in CWD
		dummyCfg := &Config{SenderCompID: "CWD_SENDER"}
		expectedPath := filepath.Join(cwdDir, ".mxrc")
		dummyCfg.dump(expectedPath)

		// Execute
		if cfg, loadedPath := initConfig(); loadedPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, loadedPath)
		} else if cfg.SenderCompID != "CWD_SENDER" {
			t.Errorf("Expected SenderCompID 'CWD_SENDER', got %s", cfg.SenderCompID)
		}
	})

	t.Run("Loads from Home if CWD does not exist but Home does", func(t *testing.T) {
		cwdDir := t.TempDir()
		homeDir := t.TempDir()

		os.Chdir(cwdDir)
		t.Setenv("HOME", homeDir)
		t.Setenv("USERPROFILE", homeDir)

		// Create a dummy config ONLY in Home directory
		dummyCfg := &Config{SenderCompID: "HOME_SENDER"}
		expectedPath := filepath.Join(homeDir, ".mxrc")
		dummyCfg.dump(expectedPath)

		// Execute
		if cfg, loadedPath := initConfig(); loadedPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, loadedPath)
		} else if cfg.SenderCompID != "HOME_SENDER" {
			t.Errorf("Expected SenderCompID 'HOME_SENDER', got %s", cfg.SenderCompID)
		}
	})
}
