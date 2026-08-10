package macros

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// Regex to find $SOMETHING or $PREFIX.SOMETHING, bracket matching is handled explictly
var varRegex = regexp.MustCompile(`\$([A-Za-z_]+)(?:\.([A-Za-z0-9_]+))?`)

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
		return substituteUnique(matchUC)
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
