package transport

import (
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
)

func TestNewMockConnection(t *testing.T) {
	bufferSize := 5
	mock := NewMockConnection(bufferSize)

	if cap(mock.IncomingCh) != bufferSize {
		t.Errorf("expected IncomingCh capacity %d, got %d", bufferSize, cap(mock.IncomingCh))
	}
	if cap(mock.OutgoingCh) != bufferSize {
		t.Errorf("expected OutgoingCh capacity %d, got %d", bufferSize, cap(mock.OutgoingCh))
	}
	if cap(mock.ErrorCh) != bufferSize {
		t.Errorf("expected ErrorCh capacity %d, got %d", bufferSize, cap(mock.ErrorCh))
	}
}

func TestMockConnection_Close(t *testing.T) {
	mock := NewMockConnection(1)

	mock.Close()
	mock.Close()

	// Verify the done channel actually closed
	select {
	case <-mock.Done(): // Success
	default:
		t.Fatal("expected Done() channel to be closed, but it blocked")
	}
}

func TestMockConnection_Drain(t *testing.T) {
	mock := NewMockConnection(10)
	defer mock.Close()

	// Start draining in the background
	go mock.Drain()

	for range 5 {
		mock.Outgoing() <- message.Message{}
	}

	// Micro-sleep to allow the background goroutine to process the select loop
	time.Sleep(10 * time.Millisecond)

	// Verify the channel was actually drained
	if len(mock.OutgoingCh) != 0 {
		t.Errorf("expected outgoing channel to be empty, but has %d messages left", len(mock.OutgoingCh))
	}
}
