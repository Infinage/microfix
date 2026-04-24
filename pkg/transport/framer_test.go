package transport

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"time"
)

func TestFrame_ValidMessages(t *testing.T) {
    // A stream containing two valid FIX messages back-to-back
    // Message 1: BodyLength=5 (35=A|)
    // Message 2: BodyLength=5 (35=0|)
    rawStream := "8=FIX.4.4|9=5|35=A|10=123|8=FIX.4.4|9=5|35=0|10=456|"
    reader := bufio.NewReader(strings.NewReader(rawStream))

    // First Message
    msg1, err := frame(reader, '|')
    if err != nil {
        t.Fatalf("Failed to read first message: %v", err)
    }
    if expected := "8=FIX.4.4|9=5|35=A|10=123|"; msg1 != expected {
        t.Errorf("Msg 1 mismatch.\nGot:  %q\nWant: %q", msg1, expected)
    }

    // Second Message
    msg2, err := frame(reader, '|')
    if err != nil {
        t.Fatalf("Failed to read second message: %v", err)
    }
    if expected := "8=FIX.4.4|9=5|35=0|10=456|"; msg2 != expected {
        t.Errorf("Msg 2 mismatch.\nGot:  %q\nWant: %q", msg2, expected)
    }
}

func TestFrame_InvalidStructure(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr string
    }{
        {
            name:    "Wrong BeginString",
            input:   "8=NOTFIX|9=5|35=A|10=123|",
            wantErr: "Invalid fix begin string",
        },
        {
            name:    "Missing BodyLength",
            input:   "8=FIX.4.4|35=A|10=123|",
            wantErr: "Expected BodyLength tag",
        },
        {
            name:    "Corrupted Trailer",
            input:   "8=FIX.4.4|9=5|35=A|XX=123|",
            wantErr: "Expected the fix message to end with checksum",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            reader := bufio.NewReader(strings.NewReader(tt.input))
            _, err := frame(reader, '|')
            if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
                t.Errorf("Expected error containing %q, got %v", tt.wantErr, err)
            }
        })
    }
}

func TestFrame_PartialData(t *testing.T) {
	// io.Pipe creates a synchronous in-memory pipe.
	// Reads block until a Write happens.
	pr, pw := io.Pipe()
	reader := bufio.NewReader(pr)

	// Start framing in a separate goroutine because it will block
	resultChan := make(chan string)
	errChan := make(chan error)
	go func() {
		msg, err := frame(reader, '|')
		t.Log("Starting async framer...")
		//time.Sleep(time.Second * 1)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- msg
	}()

	// Simulate "Slow" network: Write the header first, then later the rest
	pw.Write([]byte("8=FIX.4.4|9=5|"))
	pw.Write([]byte("35=A|10=123|"))

	select {
	case msg := <-resultChan:
		expected := "8=FIX.4.4|9=5|35=A|10=123|"
		if msg != expected {
			t.Errorf("Got %q, want %q", msg, expected)
		}
	case err := <-errChan:
		t.Fatalf("Unexpected error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out - framer got stuck!")
	}
}
