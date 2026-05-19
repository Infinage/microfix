package transport

import (
	"sync"

	"github.com/infinage/microfix/pkg/message"
)

// MockConnection simulates the underlying transport for testing purposes.
type MockConnection struct {
	IncomingCh chan message.Message
	OutgoingCh chan message.Message
	ErrorCh    chan error
	done       chan struct{}
	once       sync.Once
}

// NewMockConnection initializes a mock connection with buffered channels.
// bufferSize denotes the capacity of the channels to create.
func NewMockConnection(bufferSize int) *MockConnection {
	return &MockConnection{
		IncomingCh: make(chan message.Message, bufferSize),
		OutgoingCh: make(chan message.Message, bufferSize),
		ErrorCh:    make(chan error, bufferSize),
		done:       make(chan struct{}),
	}
}

func (m *MockConnection) Incoming() <-chan message.Message { return m.IncomingCh }
func (m *MockConnection) Outgoing() chan<- message.Message { return m.OutgoingCh }
func (m *MockConnection) Errors() <-chan error             { return m.ErrorCh }
func (m *MockConnection) Done() <-chan struct{}            { return m.done }

// Close safely shuts down the connection. It is safe to call multiple times.
func (m *MockConnection) Close() {
	m.once.Do(func() {
		close(m.done)
	})
}

// Drain runs an infinite loop until Close() is called, preventing
// deadlocks by consuming outgoing messages.
// Intended to be run via a separate goroutine.
func (con *MockConnection) Drain() {
	for {
		select {
		case <-con.OutgoingCh:
		case <-con.done:
			return
		}
	}
}
