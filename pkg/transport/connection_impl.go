package transport

import (
	"bufio"
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

	// Stop signal to close all channels and sockets
	stopFlag chan any
	stopOnce sync.Once
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

func (t *transport) Done() <-chan any {
	return t.stopFlag
}

// Helper to close socket
func (t *transport) Close() {
	t.stopOnce.Do(func() {
		close(t.stopFlag)
		t.conn.Close()
	})
}

// Helper to init connection and start event loops
func newTransport(conn net.Conn) *transport {
	t := &transport{
		conn:     conn,
		incoming: make(chan message.Message, 1024),
		outgoing: make(chan message.Message, 1024),
		errors:   make(chan error, 10),
		stopFlag: make(chan any),
	}

	go t.readLoop()
	go t.writeLoop()

	return t
}

// Read from socket and push messages to Incoming
func (t *transport) readLoop() {
	defer t.Close()
	reader := bufio.NewReader(t.conn)
	for {
		fixString, err := frame(reader, '\x01')
		if err != nil {
			select {
			case <-t.stopFlag:
				return
			default:
				t.errors <- fmt.Errorf("Failed to read: %w", err)
			}
		}

		message, err := message.MessageFromString(fixString, "\x01")
		if err != nil {
			t.errors <- fmt.Errorf("Failed to parse: %w", err)
		}

		t.incoming <- message
	}
}

// Read from Outgoing channel and write to socket
func (t *transport) writeLoop() {
	defer t.Close()
	for {
		select {
		case <-t.stopFlag:
			return
		case message, ok := <-t.outgoing:
			if !ok {
				return
			}
			wire := message.String("\x01")
			_, err := t.conn.Write([]byte(wire))
			if err != nil {
				t.errors <- fmt.Errorf("Failed to write: %w", err)
			}
		}
	}
}
