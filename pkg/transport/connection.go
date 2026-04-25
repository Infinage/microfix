package transport

import (
	"github.com/infinage/microfix/pkg/message"
	"net"
)

// Listen for messages on Incoming, send messages to Outgoing, monitor on Errors
type Connection interface {
	Incoming() <-chan message.Message
	Outgoing() chan<- message.Message
	Errors() <-chan error
	Done() <-chan struct{}
	Close()
}

// Connect to addr (ip:port)
func Dial(addr string) (Connection, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return newTransport(conn), nil
}

// Listens for a single incoming connection
func Listen1(addr string) (Connection, error) {
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
