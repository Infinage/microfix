package transport

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/infinage/microfix/pkg/message"
)

// Private struct implementing Connection interface
type transport struct {
	conn     net.Conn
	incoming chan message.Message
	outgoing chan message.Message
	errors   chan error

	// Context to close the goroutines
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (t *transport) Incoming() <-chan message.Message {
	return t.incoming
}

func (t *transport) Outgoing() chan<- message.Message {
	return t.outgoing
}

func (t *transport) Errors() <-chan error {
	return t.errors
}

func (t *transport) Done() <-chan struct{} {
	return t.ctx.Done()
}

// Signal to stop goroutines, close outgoing channels and socket
func (t *transport) Close() {
	t.once.Do(func() {
		t.cancel()
		t.conn.Close()
	})
}

// Helper to init connection and start event loops
func newTransport(conn net.Conn) *transport {
	ctx, cancel := context.WithCancel(context.Background())

	t := &transport{
		conn:     conn,
		incoming: make(chan message.Message, 1024),
		outgoing: make(chan message.Message, 1024),
		errors:   make(chan error, 10),
		ctx:      ctx,
		cancel:   cancel,
	}

	go t.readLoop()
	go t.writeLoop()

	return t
}

// Read from socket and push messages to Incoming
func (t *transport) readLoop() {
	defer close(t.incoming)
	defer t.Close()

	reader := bufio.NewReader(t.conn)
	for {
		// Read into the socket until we have a single valid FIX message
		fixString, err := frame(reader, '\x01')
		if err != nil {
			t.reportError(fmt.Errorf("Failed to read: %w", err))
			return
		}

		// Parse the framed message
		message, err := message.MessageFromString(fixString, "\x01")
		if err != nil {
			t.reportError(fmt.Errorf("Failed to parse: %w", err))
			continue
		}

		// Attempt to write if we didn't recieve the cancel signal yet
		// Will block if incoming channel is full since we dont have a default
		select {
		case t.incoming <- message:
		case <-t.ctx.Done():
			return
		}
	}
}

// Read from Outgoing channel and write to socket
func (t *transport) writeLoop() {
	defer t.Close()
	for {
		select {
		case <-t.ctx.Done():
			return
		case message, ok := <-t.outgoing:
			if !ok { // User closed the outgoing channel
				return
			}
			wire := message.String("\x01")
			if _, err := t.conn.Write([]byte(wire)); err != nil {
				t.reportError(fmt.Errorf("Failed to write: %w", err))
			}
		}
	}
}

// Non blocking send to errors channel
func (t *transport) reportError(err error) {
	select {
	case <-t.ctx.Done():
		return
	case t.errors <- err:
		// error delivered
	default:
		// Channel is full, drop the error
	}
}
