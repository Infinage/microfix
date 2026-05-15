package ringbuf

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

func TestCircularBuffer_BasicOps(t *testing.T) {
	cb := NewCircularBuffer(3)

	// Fill the buffer
	cb.Write("line 1")
	cb.Write("line 2")
	cb.Write("line 3")

	// Overwrite the first element
	cb.Write("line 4")

	// Dump returns all logs - check for circular nature
	output := cb.Dump()
	expected := []string{"line 2", "line 3", "line 4"}
	if !slices.Equal(output, expected) {
		t.Errorf("expected:\n%v\n\ngot:\n%v", expected, output)
	}

	// Clear the logs and ensure that dump yields empty
	cb.Clear()
	output = cb.Dump()
	if len(output) != 0 {
		t.Errorf("Expected buffer to be empty post clear, got:\n%v", output)
	}
}

func TestCircularBuffer_HeadTail(t *testing.T) {
	t.Run("Head and Tail - Not Wrapped", func(t *testing.T) {
		cb := NewCircularBuffer(5)
		cb.Write("msg1")
		cb.Write("msg2")
		cb.Write("msg3")

		headRes := cb.Head(2)
		if !slices.Equal(headRes, []string{"msg1", "msg2"}) {
			t.Errorf("Head 2 failed, got: %v", headRes)
		}

		tailRes := cb.Tail(2)
		if !slices.Equal(tailRes, []string{"msg2", "msg3"}) {
			t.Errorf("Tail 2 failed, got: %v", tailRes)
		}

		// Exceeding bounds (should clamp to all available)
		if exceed := cb.Head(10); !slices.Equal(exceed, []string{"msg1", "msg2", "msg3"}) {
			t.Errorf("Exceeding bound failed, got: %v", exceed)
		}
		if exceed := cb.Tail(10); !slices.Equal(exceed, []string{"msg1", "msg2", "msg3"}) {
			t.Errorf("Exceeding bound failed, got: %v", exceed)
		}

		// Zero count
		if len(cb.Head(0)) != 0 || len(cb.Tail(0)) != 0 {
			t.Errorf("Head/Tail 0 should return empty slice")
		}
	})

	t.Run("Head and Tail - Wrapped", func(t *testing.T) {
		cb := NewCircularBuffer(5)
		for i := 1; i <= 7; i++ {
			cb.Write(fmt.Sprintf("msg%d", i))
		}

		// Buffer holds: msg3, msg4, msg5, msg6, msg7
		expectedAll := []string{"msg3", "msg4", "msg5", "msg6", "msg7"}

		headRes := cb.Head(2)
		if !slices.Equal(headRes, []string{"msg3", "msg4"}) {
			t.Errorf("Head 2 failed, got: %v", headRes)
		}

		tailRes := cb.Tail(2)
		if !slices.Equal(tailRes, []string{"msg6", "msg7"}) {
			t.Errorf("Tail 2 failed, got: %v", tailRes)
		}

		// Exceeding bounds (should clamp to all)
		if exceed := cb.Head(10); !slices.Equal(exceed, expectedAll) {
			t.Errorf("Head exceeding bounds failed, got: %v", exceed)
		}
		if exceed := cb.Tail(10); !slices.Equal(exceed, expectedAll) {
			t.Errorf("Tail exceeding bounds failed, got: %v", exceed)
		}
	})
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

	t.Run("HeadTailAPI", func(t *testing.T) {
		cb := NewCircularBuffer(10)
		var wg sync.WaitGroup
		wg.Add(2)

		// Spin up a writer
		go func() {
			defer wg.Done()
			for range 100 {
				cb.Write("data")
			}
		}()

		// Spin up a reader hitting boundaries
		go func() {
			defer wg.Done()
			for range 100 {
				_ = cb.Head(5)
				_ = cb.Tail(15)
				_ = cb.Dump()
			}
		}()

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

		// Listen for logs on subscribed channels
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
		// We write 300 messages WITHOUT reading to verify the non blocking drop logic.
		for range 300 {
			cb.Write("data")
		}

		// Verify the channel is full
		if len(ch) != 256 {
			t.Errorf("Expected channel to have 256 dropped messages, got %d", len(ch))
		}
	})
}
