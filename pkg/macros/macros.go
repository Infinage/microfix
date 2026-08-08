package macros

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// Regex to find $SOMETHING or $PREFIX.SOMETHING
var varRegex = regexp.MustCompile(
	`\$([A-Za-z_]+)(?:\.([A-Za-z0-9_]+))?(?:\[([^\]]*)\])?`,
)

func extractSBrackets(raw string) ([]string, error) {
	synErr := fmt.Errorf("Invalid syntax, must be of form: `$*[...]`")

	start := strings.Index(raw, "[")
	end := strings.Index(raw, "]")
	if start == -1 || end == -1 {
		return nil, synErr
	}

	// Extract the content within [...]
	raw = raw[start+1 : end]
	if raw == "" {
		return nil, nil
	}

	// Split on ',' and trim the contents
	splits := strings.Split(raw, ",")
	for idx := range splits {
		splits[idx] = strings.TrimSpace(splits[idx])
	}

	return splits, nil
}

// upperCasePrefix converts the prefix portion of a macro
// to uppercase to support case insensitive comparisons
// For eg: "$buF[35]" -> "$BUF[35]"
func upperCasePrefix(match string) string {
	if idx := strings.IndexAny(match, ".["); idx != -1 {
		return strings.ToUpper(match[:idx]) + match[idx:]
	}
	return strings.ToUpper(match)
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Format: $UNIQUE or $UNIQUE[N]
func substituteRandom(match string) (string, error) {
	// If no args specified generate UUID
	if match == "$UNIQUE" {
		b := make([]byte, 16)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
	}

	args, err := extractSBrackets(match)
	if err != nil {
		return "", err
	} else if len(args) != 1 {
		return "", fmt.Errorf("invalid syntax %q: expected $UNIQUE|$UNIQUE[N]", match)
	}

	length, err := strconv.Atoi(args[0])
	if err != nil || length <= 0 {
		return "", fmt.Errorf("invalid length parameter %q (expected > 0): %v", args[0], err)
	}

	// Cap length at 1000
	if length > 1000 {
		length = 1000
	}

	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	var sb strings.Builder
	for i := range length {
		sb.WriteByte(charset[b[i]%byte(len(charset))])
	}

	return sb.String(), nil
}

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

func substituteDate(raw string) (string, error) {
	today := time.Now()
	if raw == "$DATE" {
		return today.Format("20060102"), nil
	}

	splits, err := extractSBrackets(raw)
	if err != nil {
		return "", err
	} else if len(splits) != 1 {
		return "", fmt.Errorf("expected format $DATE[+N], got %q", raw)
	}

	daysOffset, err := strconv.Atoi(splits[0])
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
// Supports: $UNIQUE, $TIMESTAMP, $DATE[+days], $SEQ_IN, $SEQ_OUT, $STATUS, $BUF, $ERROR,
// $LASTIN/$LASTOUT extractors and $CFG/$ALIAS/$VARS/$ENV namespaces.
//
// If quoteIfSpaces is true, resolved values containing whitespace are CSV-quoted
// so downstream tokenizers treat them as a single argument.
func Substitute(input string, sess *session.Session, st *store.Store, quoteIfSpaces bool) (string, error) {
	return subsituteAll(input, sess, st, quoteIfSpaces, make(map[string]bool))
}

// subsituteAll recursively substitutes macros whilst ensuring that there aren't any cycles
func subsituteAll(input string, sess *session.Session, st *store.Store, quoteIfSpaces bool,
	visited map[string]bool) (string, error) {
	var expandErr error

	// match is the full string: "$VAR.Symbol" or "$UNIQUE" or "$LASTIN[35]"
	result := varRegex.ReplaceAllStringFunc(input, func(match string) string {
		resolve := func() (string, error) {
			// Handle Magics (Computation)
			matchUC := upperCasePrefix(match)
			if matchUC == "$ERROR" {
				if err := st.LastError(); err != nil {
					return err.Error(), nil
				}
				return "", nil
			}
			if matchUC == "$TIMESTAMP" {
				return time.Now().UTC().Format("20060102-15:04:05.000"), nil
			}
			if strings.HasPrefix(matchUC, "$UNIQUE") {
				return substituteRandom(matchUC)
			}
			if matchUC == "$SEQ_OUT" || matchUC == "$SEQ_IN" || matchUC == "$STATUS" {
				return substituteSnapshot(matchUC, sess), nil
			}
			if strings.HasPrefix(matchUC, "$DATE") {
				return substituteDate(matchUC)
			}
			if strings.HasPrefix(matchUC, "$BUF") {
				return substituteBuffer(matchUC, st.Buffer())
			}
			if isIncoming := strings.HasPrefix(matchUC, "$LASTIN"); isIncoming || strings.HasPrefix(matchUC, "$LASTOUT") {
				return substituteMessageTag(matchUC, isIncoming, sess.LastMessage)
			}

			// Handle State (CFG, ALIAS, VARS, ENV)
			// Ensure that we aren't already trying to expand the key
			storeKey := strings.TrimPrefix(match, "$")
			if visited[storeKey] {
				return "", fmt.Errorf("circular reference detected: %q, refers back to itself", match)
			}

			// Query value from the store
			val, ok, err := st.Get(storeKey)
			if !ok || err != nil {
				return "", fmt.Errorf("variable resolution failed for %q: %w", match, err)
			}

			// Recurse deeper into the returned macro result. quoteIfSpaces is
			// set to false since we'd only want quoting at outer most lvl
			visited[storeKey] = true
			val, err = subsituteAll(val, sess, st, false, visited)
			visited[storeKey] = false
			return val, err
		}

		val, err := resolve()
		if err != nil {
			expandErr = err
			return match // preserve original text on failure
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
