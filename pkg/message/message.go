package message

import (
	"fmt"
	"iter"
	"math"
	"strconv"
	"strings"
)

type Message []Field

// MessageFromString parses a raw FIX string using the
// explicitly provided delimiter (e.g., "|", "\x01", "^").
// It expects the raw string to end with the delimiter token.
func MessageFromString(raw string, sep string) (Message, error) {
	numFields := strings.Count(raw, sep)
	if numFields == 0 {
		return nil, fmt.Errorf("No delimiters found: %v", sep)
	}

	result := make([]Field, numFields)
	for nField, field := range strings.Split(raw, sep) {
		if nField == numFields {
			if field == "" {
				break
			}
			return nil, fmt.Errorf("last token is non-empty (missing trailing delimiter %q)", sep)
		}

		// Allow values to have '=' in them, eg: tag 96 containing base 64 value
		if eqCount := strings.Count(field, "="); eqCount < 1 {
			return nil, fmt.Errorf("field missing '=' assignment operator in token: %q", field)
		}

		tagS, value, _ := strings.Cut(field, "=")
		tag, err := strconv.Atoi(tagS)
		if err != nil {
			return nil, fmt.Errorf("field tag is not an integer: %q", tagS)
		} else if limit := math.MaxUint16; tag > limit {
			return nil, fmt.Errorf("tag exceeds maximum supported limit %d: %d", limit, tag)
		}

		result[nField] = Field{uint16(tag), value}
	}

	return result, nil
}

// MessageFromStringAuto parses a raw FIX string by automatically
// inferring the delimiter from the last character of the message.
func MessageFromStringAuto(raw string) (Message, error) {
	// Minimum valid FIX string structure: "A=0|" (4 chars)
	if len(raw) < 4 {
		return nil, fmt.Errorf("FIX message must be atleast 4 chars long")
	}

	delim := raw[len(raw)-1:]
	return MessageFromString(raw, delim)
}

// Serialize to string in the Wire Format
func (msg *Message) String(sep string) string {
	if len(*msg) == 0 {
		return ""
	}
	var res []string
	for _, field := range *msg {
		res = append(res, field.ToWire())
	}
	return strings.Join(res, sep) + sep
}

// Returns the FIRST matching field value, returns false if not found
func (msg *Message) Get(tag uint16) (string, bool) {
	field, pos := msg.FindFrom(tag, 0)
	if pos == -1 {
		return "", false
	}

	return field.Value, true
}

// Modify the FIRST matching field on message, returns true if found and modified
func (msg *Message) Set(tag uint16, value string) bool {
	field, pos := msg.FindFrom(tag, 0)
	if pos == -1 {
		return false
	}

	field.Value = value
	return true
}

// Out of bound inserts are appended to the end
func (msg *Message) Insert(idx int, field Field) {
	length := len(*msg)

	// Handle out of bounds by appending to the end
	if idx < 0 || idx >= length {
		*msg = append(*msg, field)
		return
	}

	// Grow the slice by one element (append the last element again)
	*msg = append(*msg, (*msg)[length-1])

	// Shift elements to the right to make room at idx
	// This is essentially a memmove in the background
	copy((*msg)[idx+1:], (*msg)[idx:length-1])

	// Insert the new field
	(*msg)[idx] = field
}

// Searches for a tag in message from a starting index, returning
// a pointer and its index, returns -1 if not found
func (msg *Message) FindFrom(tag uint16, start int) (*Field, int) {
	for i := start; i < len(*msg); i++ {
		if (*msg)[i].Tag == tag {
			return &(*msg)[i], i
		}
	}

	return nil, -1
}

// Iterate and return all fields matching tag
func (msg *Message) FindAll(tag uint16) iter.Seq[*Field] {
	return func(yield func(*Field) bool) {
		for i := 0; i < len(*msg); i++ {
			if (*msg)[i].Tag == tag {
				if !yield(&(*msg)[i]) {
					break
				}
			}
		}
	}
}

// Checks and returns true only if all of tags are present
func (msg *Message) Contains(tags ...uint16) bool {
	if len(tags) == 0 {
		return true
	}

	var required = make(map[uint16]any)
	for _, tag := range tags {
		required[tag] = nil
	}

	for _, field := range *msg {
		delete(required, field.Tag)
	}

	return len(required) == 0
}

// Checksum of the message ignoring tag 10 if present
func (msg *Message) Checksum() uint8 {
	var result int
	for _, field := range *msg {
		if field.Tag != 10 {
			for _, ch := range field.ToWire() + "\x01" {
				result = result + int(ch)
			}
		}
	}
	return uint8(result % 256)
}

// Body length of the mesage, ignoring tags 8, 9 and 10
func (msg *Message) BodyLength() uint64 {
	var result uint64
	for _, field := range *msg {
		if !(field.Tag == 8 || field.Tag == 9 || field.Tag == 10) {
			result += uint64(len(field.ToWire())) + 1
		}
	}
	return result
}

// Add Checksum and Bodylength if missing or update it
func (msg *Message) Finalize() {
	if bodyLen := fmt.Sprint(msg.BodyLength()); !msg.Set(9, bodyLen) {
		field := Field{Tag: 9, Value: bodyLen}
		msg.Insert(1, field)
	}

	if checksum := fmt.Sprintf("%03d", msg.Checksum()); !msg.Set(10, checksum) {
		field := Field{Tag: 10, Value: checksum}
		msg.Insert(len(*msg), field)
	}
}
