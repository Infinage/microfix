package macros

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/session"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Format: $UNIQUE or $UNIQUE[N]
func substituteUnique(match string) (string, error) {
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
