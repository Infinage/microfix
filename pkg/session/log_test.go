package session

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/message"
)

func TestLogFormatting(t *testing.T) {
	// Fixed time for deterministic string testing
	now, _ := time.Parse("15:04:05.000", "08:30:00.000")

	t.Run("Message Log Formatting", func(t *testing.T) {
		msg := message.Message{message.Field{Tag: 35, Value: "A"}}
		l := newMessageLog(now, msg, true) // RECV

		got := l.String()
		wantPrefix := "[08:30:00.000] RECV <<"
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("Expected prefix %q, got %q", wantPrefix, got)
		}
		if !strings.Contains(got, "35=A") {
			t.Errorf("Log string missing message content: %s", got)
		}
	})

	t.Run("Error Log Formatting", func(t *testing.T) {
		err := errors.New("socket timeout")
		l := newErrorLog(now, err)

		got := l.String()
		want := "[08:30:00.000] ERR  !! socket timeout"
		if got != want {
			t.Errorf("\nWant: %s\nGot : %s", want, got)
		}
	})

	t.Run("Sys Event Log Formatting", func(t *testing.T) {
		l := newSysEventLog(now, "Handshake complete")

		got := l.String()
		want := "[08:30:00.000] SYS  .. Handshake complete"
		if got != want {
			t.Errorf("\nWant: %s\nGot : %s", want, got)
		}
	})

	t.Run("LogType Stringer", func(t *testing.T) {
		if LogSend.String() != "SEND" {
			t.Errorf("Expected SEND, got %s", LogSend.String())
		}
		if LogType(99).String() != "UNKN" {
			t.Errorf("Expected UNKN for invalid type, got %s", LogType(99).String())
		}
	})
}
