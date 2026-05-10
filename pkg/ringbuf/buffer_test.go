package ringbuf

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestCircularBuffer_BasicOps(t *testing.T) {
	cb := NewCircularBuffer(3)

	// Fill the buffer
	cb.Write("line 1")
	cb.Write("line 2")
	cb.Write("line 3")

	// Overwrite the first element (it's circular)
	cb.Write("line 4")

	var buf bytes.Buffer
	cb.Dump(&buf)

	output := strings.TrimSpace(buf.String())
	expected := "line 2\nline 3\nline 4"

	if output != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, output)
	}

	// Clear the logs and ensure that dump yeilds empty
	buf.Reset()
	cb.Clear()
	cb.Dump(&buf)

	if output := buf.String(); output != "" {
		t.Errorf("Expected buffer to be empty post clear, got:\n%v", output)
	}
}

func TestCircularBuffer_Filter(t *testing.T) {
	cb := NewCircularBuffer(5)
	cb.Write("ExecutionReport: New")
	cb.Write("OrderSingle: Buy")
	cb.Write("ExecutionReport: Filled")

	t.Run("Valid Match", func(t *testing.T) {
		results, err := cb.Filter("ExecutionReport")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 matches, got %d", len(results))
		}
	})

	t.Run("Invalid Regex", func(t *testing.T) {
		_, err := cb.Filter("[broken-regex")
		if err == nil {
			t.Error("expected an error for invalid regex, but got nil")
		}
	})

	t.Run("No Match", func(t *testing.T) {
		results, _ := cb.Filter("Heartbeat")
		if len(results) != 0 {
			t.Errorf("expected 0 matches, got %d", len(results))
		}
	})
}

func TestCircularBuffer_Concurrency(t *testing.T) {
	t.Run("FilterAPI", func(t *testing.T) {
		cb := NewCircularBuffer(10)
		var wg sync.WaitGroup

		// Spin up a writer
		wg.Go(func() {
			for range 100 {
				cb.Write("data")
			}
		})

		// Spin up a reader/filter
		wg.Go(func() {
			for range 100 {
				_, _ = cb.Filter("data")
			}
		})

		// Wait until both are done, shouldn't be any panics
		wg.Wait()
	})

	t.Run("SubscribeAPI", func(t *testing.T) {
		cb := NewCircularBuffer(10)

		// Multiple Subscribers
		ch1, cancel1 := cb.Subscribe()
		ch2, cancel2 := cb.Subscribe()

		var wg sync.WaitGroup
		wg.Add(3) // 1 writer, 2 readers

		// Spin up a writer
		go func() {
			defer wg.Done()
			for range 100 {
				cb.Write("data")
			}
		}()

		// Listen for logs on subscribed channel 1
		for _, ch := range []<-chan string{ch1, ch2} {
			go func(ch <-chan string) {
				defer wg.Done()
				count := 0
				for range ch {
					count++
					if count == 100 {
						break // Successfully received all messages
					}
				}
			}(ch)
		}

		wg.Wait()

		// Test memory cleanup
		cancel1()
		cancel2()

		cb.mu.Lock()
		defer cb.mu.Unlock()
		if len(cb.subs) != 0 {
			t.Errorf("Expected 0 active subscriptions after cancel, got %d", len(cb.subs))
		}
	})

	t.Run("DropBehavior_SlowReader", func(t *testing.T) {
		cb := NewCircularBuffer(10)
		ch, cancel := cb.Subscribe()
		defer cancel()

		// The subscriber channel buffer is size 256.
		// We are going to write 300 messages WITHOUT reading them.
		// If the non-blocking `select` in cb.Write is broken, this test will deadlock/freeze.
		for range 300 {
			cb.Write("data")
		}

		// Verify the channel is full
		if len(ch) != 256 {
			t.Errorf("Expected channel to have 256 dropped messages, got %d", len(ch))
		}
	})
}
