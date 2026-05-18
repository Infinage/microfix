package executor

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	script "github.com/infinage/microfix/pkg/executor/handlers"
	"github.com/infinage/microfix/pkg/message"
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

func substituteMessageTag(raw string, isIncoming bool, ctx *script.ScriptContext) (string, error) {
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
	var msg *message.Message
	if isIncoming {
		msg = ctx.Session.LastMessage(msgType, true)
	} else {
		msg = ctx.Session.LastMessage(msgType, false)
	}

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

func substituteSnapshot(raw string, ctx *script.ScriptContext) string {
	snap := ctx.Session.Status()
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

// Expand takes a string like "35=D|11=$UNIQUE|55=$VAR.Symbol" and fills it in.
// Magic vars: $UNIQUE, $TIMESTAMP, $DATE, $DATE[+days], $LASTIN[MsgType, tag], $LASTOUT[MsgType,tag]
// Store vars: $CFG.*, $ALIAS.*, $VARS.*, $ENV.*
func Substitute(input string, ctx *script.ScriptContext) (string, error) {
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
			return substituteSnapshot(match, ctx)
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
			res, err := substituteMessageTag(match, isIncoming, ctx)
			if err != nil {
				expandErr = err
				return match
			}
			return res
		}

		// Handle State (CFG, ALIAS, VARS, ENV)
		// Strip the '$' and ask the store
		storeKey := strings.TrimPrefix(match, "$")
		val, ok, err := ctx.Store.Get(storeKey)
		if !ok || err != nil {
			expandErr = fmt.Errorf("variable resolution failed for '%s': %w", match, err)
			return match
		}
		return val
	})

	return result, expandErr
}
