package session

import (
	"fmt"
	"time"

	"github.com/infinage/microfix/pkg/message"
)

type LogType int

const (
	LogSend LogType = iota // Sent FIX messages
	LogRecv                // Recieved FIX messages
	LogErr                 // Critial errors observed
	LogInfo                // Non critical informational messages
	LogTran                // State transition
)

func (typ LogType) String() string {
	var types = []string{"SEND", "RECV", "ERR ", "INFO", "TRAN"}
	if typn := int(typ); typn < 0 || typn >= len(types) {
		return "UNKN"
	}
	return types[int(typ)]
}

type Log struct {
	Type      LogType
	Timestamp time.Time
	Msg       message.Message // LogSend, LogRecv
	Err       error           // LogErr
	Text      string          // LogInfo
	States    [2]string       // LogTran
}

func (log Log) Content() string {
	switch log.Type {
	case LogRecv, LogSend:
		return log.Msg.String("|")

	case LogErr:
		return log.Err.Error()

	case LogInfo:
		return log.Text

	case LogTran:
		return fmt.Sprintf("%s -> %s", log.States[0], log.States[1])

	default:
		return ""
	}
}

// MsgName: Logon, Heartbeat, etc. Can be "", then ignored
// Only technically valid for Recv and Send
func (log Log) String(msgName string) string {
	ts := log.Timestamp.Format("2006-01-02 15:04:05.000")

	// Choose a symbol based on type
	symbol := ".."
	switch log.Type {
	case LogSend:
		symbol = ">>"
	case LogRecv:
		symbol = "<<"
	case LogErr:
		symbol = "!!"
	case LogTran:
		symbol = "::"
	}

	// Optional message name
	if msgName != "" {
		msgName = fmt.Sprintf("[%s] ", msgName)
	}

	return fmt.Sprintf("[%s] %s %s %s%s", ts, log.Type, symbol, msgName, log.Content())
}

func newErrorLog(now time.Time, err error) Log {
	return Log{Type: LogErr, Timestamp: now, Err: err}
}

func newMessageLog(now time.Time, msg message.Message, isIncoming bool) Log {
	var logType LogType
	if isIncoming {
		logType = LogRecv
	} else {
		logType = LogSend
	}

	return Log{Type: logType, Timestamp: now, Msg: msg}
}

func newInfoLog(now time.Time, text string) Log {
	return Log{Type: LogInfo, Timestamp: now, Text: text}
}

func newStateTransitionLog(now time.Time, prev, current string) Log {
	return Log{Type: LogTran, Timestamp: now, States: [2]string{prev, current}}
}
