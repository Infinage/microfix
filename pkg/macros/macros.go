package macros

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// Regex to find $SOMETHING or $PREFIX.SOMETHING
var varRegex = regexp.MustCompile(
	`\$([A-Z_]+)(?:\.([A-Za-z0-9_.]+))?(?:\[([^\]]*)\])?`,
)

// Helper to generate a random UUID
func uuid() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func extractSBrackets(raw string) (string, error) {
	synErr := fmt.Errorf("Invalid syntax, must be of form: `$*[...]`")

	start := strings.Index(raw, "[")
	end := strings.Index(raw, "]")
	if start == -1 || end == -1 {
		return "", synErr
	}

	return raw[start+1 : end], nil
}

func substituteMessageTag(raw string, isIncoming bool, sess *session.Session) (string, error) {
	contents, err := extractSBrackets(raw)
	if err != nil {
		return "", err
	}

	splits := strings.SplitN(contents, ",", 2)
	if len(splits) != 2 {
		return "", fmt.Errorf("Invalid syntax, must be of form: `$*[MsgType,Tag]`")
	}

	tag, err := strconv.ParseUint(strings.TrimSpace(splits[1]), 10, 16)
	if err != nil {
		return "", fmt.Errorf("Not a valid tag: %w", err)
	}

	msgType := strings.TrimSpace(splits[0])
	msg := sess.LastMessage(msgType, isIncoming)
	if msg == nil {
		dir := "incoming"
		if !isIncoming {
			dir = "outgoing"
		}
		return "", fmt.Errorf("No %s message of type [%v] found", dir, msgType)
	}

	val, ok := msg.Get(uint16(tag))
	if !ok {
		return "", fmt.Errorf("tag %d not found in last message type %s", tag, msgType)
	}

	return val, nil
}

func substituteDate(raw string) (string, error) {
	today := time.Now()
	if raw == "$DATE" {
		return today.Format("20060102"), nil
	}

	contents, err := extractSBrackets(raw)
	if err != nil {
		return "", err
	}

	daysOffset, err := strconv.Atoi(strings.TrimSpace(contents))
	if err != nil {
		return "", fmt.Errorf("Not a valid integer offset: '%v'", err)
	}

	return today.AddDate(0, 0, daysOffset).Format("20060102"), nil
}

func substituteSnapshot(raw string, sess *session.Session) string {
	snap := sess.Status()
	switch raw[1:] {
	case "SEQ_IN":
		return fmt.Sprint(snap.InSeqNum)
	case "SEQ_OUT":
		return fmt.Sprint(snap.OutSeqNum)
	case "STATUS":
		return snap.State.String()
	default:
		return raw
	}
}

// Substitute resolves variables in a string (e.g. "35=D|11=$UNIQUE|55=$VARS.Symbol").
//
// Supports: $UNIQUE, $TIMESTAMP, $DATE[+days], $SEQ_IN, $SEQ_OUT, $STATUS,
// $LASTIN/$LASTOUT extractors and $CFG/$ALIAS/$VARS/$ENV/$BUF namespaces.
//
// If quoteIfSpaces is true, resolved values containing whitespace are CSV-quoted
// so downstream tokenizers treat them as a single argument.
func Substitute(input string, sess *session.Session, st *store.Store, quoteIfSpaces bool) (string, error) {
	var expandErr error

	// match is the full string: "$VAR.Symbol" or "$UNIQUE" or "$LASTIN[35]"
	result := varRegex.ReplaceAllStringFunc(input, func(match string) string {
		// Handle Magics (Computation)
		if match == "$UNIQUE" {
			return uuid()
		}
		if match == "$TIMESTAMP" {
			return time.Now().UTC().Format("20060102-15:04:05.000")
		}
		if match == "$SEQ_OUT" || match == "$SEQ_IN" || match == "$STATUS" {
			return substituteSnapshot(match, sess)
		}
		if strings.HasPrefix(match, "$DATE") {
			res, err := substituteDate(match)
			if err != nil {
				expandErr = err
				return match
			}
			return res
		}
		if isIncoming := strings.HasPrefix(match, "$LASTIN"); isIncoming || strings.HasPrefix(match, "$LASTOUT") {
			res, err := substituteMessageTag(match, isIncoming, sess)
			if err != nil {
				expandErr = err
				return match
			}
			return res
		}

		// Handle State (CFG, ALIAS, VARS, ENV, BUF)
		// Strip the '$' and ask the store
		storeKey := strings.TrimPrefix(match, "$")
		val, ok, err := st.Get(storeKey)
		if !ok || err != nil {
			expandErr = fmt.Errorf("variable resolution failed for '%s': %w", match, err)
			return match
		}

		// Enclose multi word strings inside quotes for a CSV reader to understand
		if quoteIfSpaces && strings.ContainsAny(val, " \t\r\n") {
			val = strings.ReplaceAll(val, `"`, `""`)
			val = `"` + val + `"`
		}

		return val
	})

	return result, expandErr
}
