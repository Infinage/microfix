package ringbuf

import (
	"regexp"
	"sync"
)

// --- Circular Buffer ---
type CircularBuffer struct {
	lines  []string
	size   int
	mu     sync.Mutex
	ptr    int
	isFull bool
	subs   map[chan string]any
}

func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		lines: make([]string, size),
		size:  size,
		subs:  make(map[chan string]any),
	}
}

func (cb *CircularBuffer) Write(msg string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.lines[cb.ptr] = msg
	cb.ptr++

	// Wrap around
	if cb.ptr == cb.size {
		cb.isFull = true
		cb.ptr = 0
	}

	for ch := range cb.subs {
		select {
		case ch <- msg: // Delivered sucessfully
		default: // Drop if channel is full
		}
	}
}

// Dump all of the buffer contents
func (cb *CircularBuffer) Dump() []string {
	var result []string
	for _, line := range cb.Head(cb.size) {
		result = append(result, line)
	}
	return result
}

// Returns the first N elements
func (cb *CircularBuffer) Head(n int) []string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.extract(0, n)
}

// Returns the last N elements
func (cb *CircularBuffer) Tail(n int) []string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	length := cb.ptr
	if cb.isFull {
		length = cb.size
	}

	return cb.extract(length-n, length)
}

// Search for a pattern across the buffer and return all matching lines
func (cb *CircularBuffer) Filter(pattern string) ([]string, error) {
	var results []string
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return results, err
	}

	for _, line := range cb.Dump() {
		if re.MatchString(line) {
			results = append(results, line)
		}
	}

	return results, nil
}

// Clear all logs from buffer
func (cb *CircularBuffer) Clear() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.ptr, cb.isFull = 0, false
	for idx := range cb.size {
		cb.lines[idx] = ""
	}
}

// ASSUMES CALLER HOLDS THE MUTEX LOCK
func (cb *CircularBuffer) extract(begin, end int) []string {
	startIdx, length := 0, cb.ptr
	if cb.isFull {
		startIdx = cb.ptr
		length = cb.size
	}

	// Clip the indices within bounds
	begin, end = max(begin, 0), min(end, length)
	var result []string
	if begin >= end {
		return result
	}

	for i := begin; i < end; i++ {
		index := (startIdx + i) % cb.size
		result = append(result, cb.lines[index])
	}

	return result
}
