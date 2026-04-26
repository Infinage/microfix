package main

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// --- Circular Buffer for Silent Logging ---

type CircularBuffer struct {
	lines []string
	size  int
	mu    sync.Mutex
	ptr   int
}

func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		lines: make([]string, size),
		size:  size,
	}
}

func (cb *CircularBuffer) Write(msg string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.lines[cb.ptr] = fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05.000"), msg)
	cb.ptr = (cb.ptr + 1) % cb.size
}

func (cb *CircularBuffer) Dump(w io.Writer) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	for i := 0; i < cb.size; i++ {
		index := (cb.ptr + i) % cb.size
		if cb.lines[index] != "" {
			fmt.Fprintln(w, cb.lines[index])
		}
	}
}
