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
	var types = []string{"SEND", "RECV", "ERR", "SYS"}
	if typn := int(typ); typn < 0 || typn >= len(types) {
		return "UNKN"
	}
	return types[int(typ)]
}

type Log struct {
	Type      LogType
	Timestamp time.Time
	Msg       message.Message
	Err       error
	Text      string
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

func (log Log) String() string {
	ts := log.Timestamp.Format("15:04:05.000")

	switch log.Type {
	case LogRecv:
		return fmt.Sprintf("[%s] RECV << %s", ts, log.Content())

	case LogSend:
		return fmt.Sprintf("[%s] SEND >> %s", ts, log.Content())

	case LogErr:
		return fmt.Sprintf("[%s] ERR  !! %v", ts, log.Content())

	case LogSys:
		return fmt.Sprintf("[%s] SYS  .. %v", ts, log.Content())

	default:
		return fmt.Sprintf("[%s] UNKN ?? Unexpected log type", ts)
	}
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
