package session

import (
	"fmt"
	"time"

	"github.com/infinage/microfix/pkg/message"
)

type LogType int

const (
	LogSend LogType = iota
	LogRecv
	LogErr
	LogSys
)

func (typ LogType) String() string {
	var types = []string{"SEND", "RECV", "ERR ", "SYS "}
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
	Text      string          // LogSys
}

func (log Log) Content() string {
	switch log.Type {
	case LogRecv, LogSend:
		return log.Msg.String("|")

	case LogErr:
		return log.Err.Error()

	case LogSys:
		return log.Text

	default:
		return ""
	}
}

// MsgName: Logon, Heartbeat, etc. Can be "", then ignored
// Only technically valid for Recv and Send
func (log Log) String(msgName string) string {
	ts := log.Timestamp.Format("15:04:05.000")

	// Choose a symbol based on type
	symbol := ".."
	switch log.Type {
	case LogSend:
		symbol = ">>"
	case LogRecv:
		symbol = "<<"
	case LogErr:
		symbol = "!!"
	}

	// Get the MsgName (Logon, Heartbeat, etc.)
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

func newSysEventLog(now time.Time, text string) Log {
	return Log{Type: LogSys, Timestamp: now, Text: text}
}
