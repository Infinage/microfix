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

// Regex to find $SOMETHING or $PREFIX.SOMETHING, bracket matching is handled explictly
var varRegex = regexp.MustCompile(`\$([A-Za-z_]+)(?:\.([A-Za-z0-9_]+))?`)

// findMatchingBracket loops through an input string from a given position
// and returns the index at which a matching closing bracket is found
func findMatchingBracket(input string, start int) int {
	depth := 0
	for i := start; i < len(input); i++ {
		switch input[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// extractSBrackets: simple utility to split comma separated values inside [...]
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

// splitStoreKey strips the dollar prefix and returns
// params inside [..] if found as second return value
func splitStoreKey(raw string) (key, params string) {
	key = strings.TrimPrefix(strings.TrimSpace(raw), "$")
	idx := strings.Index(key, "[")
	if N := len(key); idx > 0 && key[N-1] == ']' {
		key, params = key[:idx], key[idx+1:N-1]
	}
	return key, params
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

// subsituteMessageWithParamString substitutes values inside 'val' by parsing
// contents of 'paramStr'. Syntax: "tag1=value,tag2.instance=value1\,value2,..."
//
// If unescapeCommas is true, we are at the outermost call stack and need to unescape
// commas. We will not bother escaping commas here, it is the user's responsibility to
// ensure that they are escaped as appropriate if they intend to use it recursively.
func substituteMessageWithParamString(raw, paramStr string, unescapeCommas bool) (string, error) {
	if paramStr == "" {
		if unescapeCommas {
			return strings.ReplaceAll(raw, "\\,", ","), nil
		}
		return raw, nil
	}

	msg, err := message.MessageFromStringAuto(raw)
	if err != nil {
		return "", err
	}

	// Note: instance is 0 indexed but user enters starting at 1
	type param struct {
		tag      uint16
		instance int
		value    string
	}

	// Duplicates are disallowed
	paramKeys := make(map[string]bool)

	// Split by unescaped commas
	var params []param
	start, isKeyPortion := 0, true
	for idx, ch := range paramStr {
		// Parsing key: 'tag.instance', ends with an equal sign
		if isKeyPortion && ch == '=' {
			paramKey := paramStr[start:idx]
			if paramKeys[paramKey] {
				return "", fmt.Errorf("duplicate parameter keys are not allowed: %q", paramKey)
			}

			tagStr, instStr, ok := strings.Cut(paramKey, ".")
			if ok && instStr == "" {
				return "", fmt.Errorf("invalid syntax: key has a trailing '.': %q", paramKey)
			}

			tag, err := strconv.ParseUint(tagStr, 10, 16)
			if err != nil {
				return "", fmt.Errorf("invalid tag %s", tagStr)
			}

			// assume instance value as 1 if not provided
			inst := 1
			if instStr != "" {
				if inst, err = strconv.Atoi(instStr); err != nil || inst <= 0 {
					return "", fmt.Errorf("invalid instance %s (expected >= 1): %v", instStr, err)
				}
			}

			paramKeys[fmt.Sprintf("%d.%d", tag, inst)] = true
			params = append(params, param{tag: uint16(tag), instance: inst - 1})
			isKeyPortion = false
			start = idx + 1
			continue
		}

		// Parsing value: 'value', ends with a comma
		if !isKeyPortion && ch == ',' {
			// Ignore escaped commas
			if idx > 0 && paramStr[idx-1] == '\\' {
				continue
			}

			params[len(params)-1].value = paramStr[start:idx]
			isKeyPortion = true
			start = idx + 1
			continue
		}
	}

	// Flush the final value after the loop ends
	if isKeyPortion {
		return "", fmt.Errorf("missing '=value' for parameter ending at %q", paramStr[start:])
	}
	params[len(params)-1].value = paramStr[start:]

	// Store all the positions of various tags for a quicker lookup
	positions := make(map[uint16][]int)
	for pos, field := range msg {
		positions[field.Tag] = append(positions[field.Tag], pos)
	}

	// Substitute message contents with parsed pieces from ParamStr
	for _, kv := range params {
		positionsForTag := positions[kv.tag]
		if kv.instance >= len(positionsForTag) {
			return "", fmt.Errorf("tag %d (instance %d) not found in alias payload", kv.tag, kv.instance+1)
		}
		msg[positionsForTag[kv.instance]].Value = kv.value
	}

	// Construct the FIX string back
	val := msg.String(raw[len(raw)-1:])

	// Unescape the commas if we are the outermost recursive stack
	if unescapeCommas {
		val = strings.ReplaceAll(val, "\\,", ",")
	}

	return val, nil
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

// subsituteAll recursively substitutes macros whilst ensuring that there aren't any cycles
func substituteAll(input string, sess *session.Session, st *store.Store, quoteIfSpaces bool,
	visited map[string]bool) (string, error) {

	var sb strings.Builder
	for idx := 0; idx < len(input); {
		// iterating to resolving the next match until idx exceeds len(input)
		loc := varRegex.FindStringIndex(input[idx:])
		if loc == nil { // nothing left to substitute, return left overs
			sb.WriteString(input[idx:])
			break
		}

		matchStart, matchEnd := idx+loc[0], idx+loc[1]
		sb.WriteString(input[idx:matchStart]) // plain text before this macro

		// match will contain full string: "$VAR.Symbol", "$UNIQUE" or "$LASTIN[35]"
		match := input[matchStart:matchEnd]

		// If a bracket suffix follows immediately, find its true matching close
		if matchEnd < len(input) && input[matchEnd] == '[' {
			closeIdx := findMatchingBracket(input, matchEnd)
			if closeIdx == -1 {
				return "", fmt.Errorf("unterminated bracket in %q", input[matchStart:])
			}
			match = input[matchStart : closeIdx+1]
			idx = closeIdx + 1
		} else {
			idx = matchEnd
		}

		// recursively resolve the full macro
		val, err := resolveMacro(match, sess, st, visited)
		if err != nil {
			return match, err // preserve original text on failure
		}

		// Enclose multi word strings inside quotes for a CSV reader to understand
		if quoteIfSpaces && strings.ContainsAny(val, " \t\r\n") {
			val = strings.ReplaceAll(val, `"`, `""`)
			val = `"` + val + `"`
		}
		sb.WriteString(val)
	}

	return sb.String(), nil
}

// resolveMacro helps in resolving a single macro, calls back substituteAll to resolve nested components
func resolveMacro(match string, sess *session.Session, st *store.Store, visited map[string]bool) (string, error) {
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

	// We strip out the params from key also removing the '$' prefix
	// Store is always queried without the dollar prefix
	// Eg: $alias.TR[112.1=$vars.ReqID] => ("alias.TR", "112.1=$vars.ReqID")
	storeKey, paramStr := splitStoreKey(matchUC)
	namespace, _, _ := strings.Cut(storeKey, ".")
	if namespace != "ALIAS" && paramStr != "" {
		return "", fmt.Errorf("params only supported for ALIAS, got %q", matchUC)
	} else if strings.Contains(storeKey, "[") {
		return "", fmt.Errorf("params must end with ']', got %q", matchUC)
	}

	// Ensure that we aren't already trying to expand the key
	if visited[storeKey] {
		return "", fmt.Errorf("circular reference detected: %q, refers back to itself", matchUC)
	}

	// Query value from the store (note that store.Get is case insensitive)
	val, ok, err := st.Get(storeKey)
	if !ok || err != nil {
		return "", fmt.Errorf("variable resolution failed for %q: %w", matchUC, err)
	}

	// Recurse deeper into the returned macro result. quoteIfSpaces is
	// set to false since we'd only want quoting at outer most lvl
	visited[storeKey] = true
	val, err = substituteAll(val, sess, st, false, visited)
	if err == nil {
		paramStr, err = substituteAll(paramStr, sess, st, false, visited)
	}
	delete(visited, storeKey)

	// At this point if 'paramStr' is non empty, val is an alias
	if paramStr != "" && err == nil {
		val, err = substituteMessageWithParamString(val, paramStr, len(visited) == 0)
		if err != nil {
			return "", fmt.Errorf("substitution failed %q: %w", matchUC, err)
		}
	}

	return val, err
}

// Substitute resolves variables in a string (e.g. "35=D|11=$UNIQUE|55=$VARS.Symbol").
//
// Supports: $UNIQUE, $TIMESTAMP, $DATE[+days], $SEQ_IN, $SEQ_OUT, $STATUS, $BUF, $ERROR,
// $LASTIN/$LASTOUT extractors and $CFG/$ALIAS/$VARS/$ENV namespaces.
//
// If quoteIfSpaces is true, resolved values containing whitespace are CSV-quoted
// so downstream tokenizers treat them as a single argument.
func Substitute(input string, sess *session.Session, st *store.Store, quoteIfSpaces bool) (string, error) {
	return substituteAll(input, sess, st, quoteIfSpaces, make(map[string]bool))
}
