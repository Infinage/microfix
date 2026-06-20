package tfilelogger

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type LogStore struct {
	file *os.File
	mu   sync.Mutex
}

func NewTempFileLogger() (*LogStore, error) {
	fp, err := os.CreateTemp("", "microfix_stream_*.log")
	if err != nil {
		return nil, fmt.Errorf("failed to create tempfile for logging: %w", err)
	}
	return &LogStore{file: fp}, nil
}

// Deletes the temp file from disk
func (f *LogStore) Cleanup() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file != nil {
		fpath := f.file.Name()
		f.file.Close()
		os.Remove(fpath)
	}
}

// Writes a new log line to temp file
func (f *LogStore) Log(log string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.file.WriteString(log); err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}
	return nil
}

// Copies the entire temp file contents to the provided writer
func (f *LogStore) DumpTo(w io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Seek to the beginning of the temp file before copying
	if _, err := f.file.Seek(0, 0); err != nil {
		return err
	}

	_, err := io.Copy(w, f.file)
	return err
}

// Copies entire file contents to provided path
func (f *LogStore) Dump(dest string) error {
	fp, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create file for dumping log: %w", err)
	}
	defer fp.Close()
	return f.DumpTo(fp)
}
