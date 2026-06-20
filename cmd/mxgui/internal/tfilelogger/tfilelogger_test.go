package tfilelogger

import (
	"os"
	"path"
	"testing"
)

func TestTempFileLogger(t *testing.T) {
	ofpath := path.Join(t.TempDir(), "dump.log")

	tlogger, err := NewTempFileLogger()
	if err != nil {
		t.Fatalf("Failed to init logger: %s", err.Error())
	}

	for range 5 {
		if err := tlogger.Log("Test1"); err != nil {
			t.Errorf("Log append failed: %s", err.Error())
		}
	}

	if err := tlogger.Dump(ofpath); err != nil {
		t.Fatalf("Failed to dump contents to file: %s", err.Error())
	}

	tfpath := tlogger.file.Name()
	tlogger.Cleanup()
	if _, err := os.Stat(tfpath); !os.IsNotExist(err) {
		t.Errorf("Failed to cleanup temp file")
	}
}
