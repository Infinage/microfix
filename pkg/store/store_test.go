package store

import (
	"path"
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

// Helper to create an isolated store for testing without hitting real ~/.mxrc
func setupTestStore(t *testing.T) *Store {
	tempDir := t.TempDir()
	cfgPath := path.Join(tempDir, "test.mxrc")

	// Manually construct the private config for test isolation
	testCfg := &Config{
		SenderCompID: "TESTSENDER",
		Port:         1234,
		Alias: map[string]string{
			"Logon": "35=A|98=0|",
		},
	}

	return &Store{
		cfg:        testCfg,
		configPath: cfgPath,
		vars:       make(map[string]string),
	}
}

func TestStore_Get(t *testing.T) {
	s := setupTestStore(t)

	// Mock environment variable
	t.Setenv("MFIX_TEST_ENV", "hello_world")

	// Inject a temporary runtime variable
	s.vars["SessionID"] = "12345"

	tests := []struct {
		name        string
		key         string
		expectedVal string
		expectFound bool
		expectErr   bool
	}{
		{"Valid CFG", "CFG.SenderCompID", "TESTSENDER", true, false},
		{"Valid ALIAS", "ALIAS.Logon", "35=A|98=0|", true, false},
		{"Valid VARS", "VARS.SessionID", "12345", true, false},
		{"Valid ENV", "ENV.MFIX_TEST_ENV", "hello_world", true, false},

		{"Missing CFG Field", "CFG.Missing", "", false, true},
		{"Missing ALIAS", "ALIAS.Missing", "", false, false},
		{"Missing VARS", "VARS.Missing", "", false, false},
		{"Missing ENV", "ENV.Missing", "", false, false},

		{"Invalid Key Format", "CFG_SenderCompID", "", false, true}, // Missing dot
		{"Unknown Prefix", "SYS.Info", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found, err := s.Get(tt.key)
			if (err != nil) != tt.expectErr {
				t.Errorf("Get(%q) error = %v, expectErr %v", tt.key, err, tt.expectErr)
				return
			}
			if found != tt.expectFound {
				t.Errorf("Get(%q) found = %v, want %v", tt.key, found, tt.expectFound)
			}
			if val != tt.expectedVal {
				t.Errorf("Get(%q) val = %q, want %q", tt.key, val, tt.expectedVal)
			}
		})
	}
}

func TestStore_Set(t *testing.T) {
	s := setupTestStore(t)

	t.Run("Set VARS (New Insert)", func(t *testing.T) {
		oldVal, existed, err := s.Set("VARS.Foo", "Bar")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if existed {
			t.Error("Expected existed to be false for new VARS insert")
		}
		if oldVal != "" {
			t.Errorf("Expected old value to be empty, got %q", oldVal)
		}
	})

	t.Run("Set VARS (Update)", func(t *testing.T) {
		oldVal, existed, err := s.Set("VARS.Foo", "Baz")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !existed {
			t.Error("Expected existed to be true for VARS update")
		}
		if oldVal != "Bar" {
			t.Errorf("Expected old value to be 'Bar', got %q", oldVal)
		}
	})

	t.Run("Set ENV (Protected)", func(t *testing.T) {
		_, _, err := s.Set("ENV.PATH", "/dev/null")
		if err == nil {
			t.Error("Expected an error when attempting to modify ENV, got nil")
		}
	})

	t.Run("Set CFG", func(t *testing.T) {
		oldVal, existed, err := s.Set("CFG.Port", "9999")
		if err != nil {
			t.Fatalf("Unexpected error setting CFG: %v", err)
		}
		if !existed {
			t.Error("CFG fields should always return true for 'existed'")
		}
		if oldVal != "1234" {
			t.Errorf("Expected old value '1234', got %q", oldVal)
		}
	})

	t.Run("Set ALIAS (Triggers Auto-Save)", func(t *testing.T) {
		oldVal, existed, err := s.Set("ALIAS.Logon", "35=A|108=30|")
		if err != nil {
			t.Fatalf("Unexpected error setting ALIAS: %v", err)
		}
		if !existed {
			t.Error("Expected ALIAS.Logon to already exist")
		}
		if oldVal != "35=A|98=0|" {
			t.Errorf("Expected old value '35=A|98=0|', got %q", oldVal)
		}
	})
}

func TestStore_Unset(t *testing.T) {
	s := setupTestStore(t)
	s.vars["TempVar"] = "123"

	t.Run("Unset VARS", func(t *testing.T) {
		val, existed, err := s.Unset("VARS.TempVar")
		if err != nil || !existed || val != "123" {
			t.Errorf("Unset VARS failed: val=%q, existed=%v, err=%v", val, existed, err)
		}

		// Verify it's actually gone
		_, found, _ := s.Get("VARS.TempVar")
		if found {
			t.Error("VARS.TempVar should have been deleted")
		}
	})

	t.Run("Unset ALIAS", func(t *testing.T) {
		val, existed, err := s.Unset("ALIAS.Logon")
		if err != nil || !existed || val != "35=A|98=0|" {
			t.Errorf("Unset ALIAS failed: val=%q, existed=%v, err=%v", val, existed, err)
		}
	})

	t.Run("Unset CFG (Protected)", func(t *testing.T) {
		_, _, err := s.Unset("CFG.Port")
		if err == nil {
			t.Error("Expected error when attempting to unset CFG")
		}
	})
}

func TestStore_ConfigCopy(t *testing.T) {
	s := setupTestStore(t)
	cfgCopy := s.Config()

	// Modify the copy (assuming setField is unexported in your real config struct)
	_, _ = cfgCopy.setField("SenderCompID", "MALICIOUS_SENDER")

	// Verify the original in the store was NOT modified
	if s.cfg.SenderCompID == "MALICIOUS_SENDER" {
		t.Error("Store.Config() leaked a reference instead of returning a copy!")
	}
}

// Ensure you have a mock or standard way to initialize a message.Message in your test
func TestStore_Buffer(t *testing.T) {
	s := setupTestStore(t)

	t.Run("Get from empty buffer", func(t *testing.T) {
		if buf := s.Buffer(); len(buf) > 0 {
			t.Errorf("Expected empty buffer, got %v", buf)
		} else if val, found, err := s.Get("BUF.35"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		} else if found || val != "" {
			t.Errorf("Expected empty buffer to return not found, got %q", val)
		}
	})

	t.Run("Invalid tag format", func(t *testing.T) {
		_, _, err := s.Get("BUF.InvalidTag")
		if err == nil {
			t.Error("Expected error when parsing non-integer tag, got nil")
		}
	})

	t.Run("Set API is protected", func(t *testing.T) {
		_, _, err := s.Set("BUF.35", "D")
		if err == nil {
			t.Error("Expected an error when attempting to modify BUF via Set, got nil")
		}
	})

	t.Run("Set and Get buffer", func(t *testing.T) {
		original := "8=FIX.4.4|35=D|"
		msg, err := message.MessageFromString(original, "|")
		if err != nil {
			t.Fatalf("Failed to parse message: %v", err)
		}

		s.SetBuffer(msg)

		buf := s.Buffer()
		if got := buf.String("|"); got != original {
			t.Errorf("Buffer contents doesn't match, want %v but got %v", original, got)
		}

		if got, ok, err := s.Get("BUF.35"); !ok || got != "D" {
			t.Errorf("Expected GET[35] to return 'D', got '%s'", got)
		} else if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if got, ok, err := s.Get("BUF.10"); ok {
			t.Errorf("Expected GET[10] to return empty, got %s", got)
		} else if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}
