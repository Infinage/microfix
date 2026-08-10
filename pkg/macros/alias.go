package macros

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/infinage/microfix/pkg/message"
)

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
