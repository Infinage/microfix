package transport

import (
	"bufio"
	"fmt"
	"net"
	"sync"

	"github.com/infinage/microfix/pkg/message"
)

// Listen for messages on Incoming, send messages to Outgoing, monitor on Errors
type Transport struct {
	conn     net.Conn
	Incoming chan message.Message
	Outgoing chan message.Message
	Errors   chan error

	// Stop signal to close all channels and sockets
	stopFlag chan any
	stopOnce sync.Once
}

// Helper to init connection and start event loops
func newTransport(conn net.Conn) *Transport {
	t := &Transport{
		conn:     conn,
		Incoming: make(chan message.Message),
		Outgoing: make(chan message.Message),
		Errors:   make(chan error),
	}

	go t.readLoop()
	go t.writeLoop()

	return t
}

// Connect to addr (ip:port)
func Dial(addr string) (*Transport, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return newTransport(conn), nil
}

// Listens for a single incoming connection
func Listen1(addr string) (*Transport, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	listener.Close()
	return newTransport(conn), nil
}

// Read from socket and push messages to Incoming
func (t *Transport) readLoop() {
	defer t.Close()
	reader := bufio.NewReader(t.conn)
	for {
		fixString, err := frame(reader, '\x01')
		if err != nil {
			select {
			case <-t.stopFlag:
				return
			default:
				t.Errors <- fmt.Errorf("Failed to read: %w", err)
				return
			}
		}

		message, err := message.MessageFromString(fixString, "\x01")
		if err != nil {
			t.Errors <- fmt.Errorf("Failed to parse: %w", err)
			return
		}

		t.Incoming <- message
	}
}

// Read from Outgoing channel and write to socket
func (t *Transport) writeLoop() {
	defer t.Close()
	for {
		select {
		case <-t.stopFlag:
			return
		case message, ok := <-t.Outgoing:
			if !ok {
				return
			}
			wire := message.String("\x01")
			_, err := t.conn.Write([]byte(wire))
			if err != nil {
				t.Errors <- fmt.Errorf("Failed to write: %w", err)
				return
			}
		}
	}
}

// Helper to close socket
func (t *Transport) Close() error {
	t.stopOnce.Do(func() {
		close(t.stopFlag)
		t.conn.Close()
	})
	return nil
}
