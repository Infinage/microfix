package macros

import (
	"fmt"

	"github.com/infinage/microfix/pkg/message"
)

// Basic format: $BUF[Tag], $BUF[Tag,Instance].
// Strings slicing: $BUF[Tag,Instance,End], $BUF[Tag,Instance,Start,End]
func substituteBuffer(raw string, msg message.Message) (string, error) {
	if raw == "$BUF" {
		if len(msg) == 0 {
			return "", fmt.Errorf("buffer is not set")
		}
		return msg.String("|"), nil
	}

	splits, err := extractSBrackets(raw)
	if err != nil {
		return "", err
	}

	if N := len(splits); N < 1 || N > 4 {
		return "", fmt.Errorf("invalid syntax %q: expected $BUF[Tag[,Instance[,End]|,Start,End]]", raw)
	}

	// Syntax errors are highlighted before we check for buffer being set
	markers, err := extractMessageMarkers(splits)
	if err != nil {
		return "", err
	}

	if len(msg) == 0 {
		return "", fmt.Errorf("buffer is not set")
	}

	return markers.extract(msg)
}

func substituteMessageTag(raw string, isIncoming bool, LastMsgFn func(string, bool) *message.Message) (string, error) {
	splits, err := extractSBrackets(raw)
	if err != nil {
		return "", err
	}

	if N := len(splits); N < 2 || N > 5 {
		return "", fmt.Errorf("invalid syntax %q: expected $[MsgType,Tag[,Instance[,End]|,Start,End]]", raw)
	}

	// extractMessageMarkers only parses from Tag, so we drop the MsgType
	markers, err := extractMessageMarkers(splits[1:])
	if err != nil {
		return "", err
	}

	dir := "incoming"
	if !isIncoming {
		dir = "outgoing"
	}

	msgType := splits[0]
	msg := LastMsgFn(msgType, isIncoming)
	if msg == nil {
		return "", fmt.Errorf("no %s message of type '%s' found in session history", dir, msgType)
	}

	// Iterate through until we reach the required instance no
	return markers.extract(*msg)
}
