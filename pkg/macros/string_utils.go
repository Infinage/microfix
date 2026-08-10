package macros

import (
	"fmt"
	"strings"
)

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
