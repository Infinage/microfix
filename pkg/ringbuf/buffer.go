package ringbuf

import (
	"fmt"
	"io"
	"regexp"
	"sync"
)

// --- Circular Buffer ---
type CircularBuffer struct {
	lines []string
	size  int
	mu    sync.Mutex
	ptr   int
	subs  map[chan string]any
}

func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		lines: make([]string, size),
		size:  size,
		subs:  make(map[chan string]any),
	}
}

// Returns a read only channel and a cancel function
func (cb *CircularBuffer) Subscribe() (<-chan string, func()) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Create a listen only channel for subscriber
	ch := make(chan string, 256)
	cb.subs[ch] = nil

	// Closure manages the scope
	unsubscribe := func() {
		cb.mu.Lock()
		defer cb.mu.Unlock()
		if _, exists := cb.subs[ch]; exists {
			delete(cb.subs, ch)
			close(ch)
		}
	}

	return ch, unsubscribe
}

func (cb *CircularBuffer) Write(msg string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.lines[cb.ptr] = msg
	cb.ptr = (cb.ptr + 1) % cb.size

	// Broadcast to active subscribers
	for ch := range cb.subs {
		select {
		case ch <- msg: // Delivered sucessfully
		default: // Drop if channel is full
		}
	}
}

// Dump all of the buffer contents into writer object
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

// Search for a pattern across the buffer and return all matching lines
func (cb *CircularBuffer) Filter(pattern string) ([]string, error) {
	var results []string
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return results, err
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	found := 0
	for i := 0; i < cb.size; i++ {
		idx := (cb.ptr + i) % cb.size
		line := cb.lines[idx]
		if line != "" && re.MatchString(line) {
			results = append(results, line)
			found++
		}
	}

	return results, nil
}

// Clear all logs from buffer
func (cb *CircularBuffer) Clear() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	for idx := range cb.size {
		cb.lines[idx] = ""
	}
}
