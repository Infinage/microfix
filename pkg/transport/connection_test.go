package transport

import (
	"net"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
)

func TestTransport_Integration(t *testing.T) {
	// In memory two way mock "sockets"
	serverConn, clientConn := net.Pipe()

	// Create server and client
	server := newTransport(serverConn)
	client := newTransport(clientConn)

	msgRaw := "8=FIX.4.4|9=75|35=A|34=1092|" +
		"49=TESTBUY1|52=20180920-18:24:59.643|56=TESTSELL1|98=0|108=60|10=178|"
	testMsg, _ := message.MessageFromString(msgRaw, "|")

	// Send from client side
	go func() {
		client.Outgoing <- testMsg
	}()

	// Validate from server side
	select {
	case recieved := <-server.Incoming:
		if val, err := recieved.Code(); err != nil || val != "A" {
			t.Error("Expected to have MsgType [35] = A")
		} else if got := recieved.String("|"); got != msgRaw {
			t.Errorf("Expected serialized message to be %v, got %v", msgRaw, got)
		}
	case err := <-server.Errors:
		t.Errorf("Server error: %v", err)
	case <-time.After(time.Second):
		t.Errorf("Timed out waiting for a message")
	}
}
