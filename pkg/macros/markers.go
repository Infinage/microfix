package macros

import (
	"fmt"
	"strconv"

	"github.com/infinage/microfix/pkg/message"
)

type messageMarkers struct {
	tag      uint16
	instance int
	start    int
	end      int
}

// Basic format: $[Tag], $[Tag,Instance].
// String slicing: $[Tag,Instance,End], $[Tag,Instance,Start,End]
func extractMessageMarkers(args []string) (markers messageMarkers, err error) {
	nargs := len(args)

	tag, err := strconv.ParseUint(args[0], 10, 16)
	if err != nil {
		return markers, fmt.Errorf("invalid tag %q: %v", args[0], err)
	}
	markers.tag = uint16(tag)

	markers.instance = 1
	if nargs > 1 {
		markers.instance, err = strconv.Atoi(args[1])
		if err != nil || markers.instance <= 0 {
			return markers, fmt.Errorf("invalid instance count %q (expected > 0): %v", args[1], err)
		}
	}

	markers.start = 0
	if nargs == 4 {
		markers.start, err = strconv.Atoi(args[2])
		if err != nil || markers.start < 0 {
			return markers, fmt.Errorf("invalid start index %q (expected >= 0): %v", args[2], err)
		}
	}

	markers.end = -1
	if nargs == 3 || nargs == 4 {
		endStr := ""
		switch nargs {
		case 3:
			endStr = args[2]
		case 4:
			endStr = args[3]
		}
		if markers.end, err = strconv.Atoi(endStr); err != nil || markers.end < -1 {
			return markers, fmt.Errorf("invalid end index %q (expected >= -1): %v", endStr, err)
		}
	}

	return markers, nil
}

// extract retrieves the specified tag, instance and slices it with provided start, end
func (m messageMarkers) extract(msg message.Message) (string, error) {
	val, count := "", m.instance
	for field := range msg.FindAll(m.tag) {
		count--
		if count == 0 {
			val = field.Value
			break
		}
	}

	if count > 0 {
		return "", fmt.Errorf("tag %d (instance %d) not found", m.tag, m.instance)
	}

	// Add bounds checking, (don't combine below to if-else, will break)
	valLen := len(val)
	if m.end == -1 || m.end > valLen {
		m.end = valLen
	}
	if m.start > valLen {
		m.start = valLen
	}
	if m.start > m.end {
		m.start = m.end
	}

	return val[m.start:m.end], nil
}
