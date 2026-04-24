package message

import (
	"fmt"
	"iter"
	"strconv"
	"strings"
)

type Message []Field

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
			return nil, fmt.Errorf("Last token is non empty")
		}

		eqCount := strings.Count(field, "=")
		if eqCount != 1 {
			return nil, fmt.Errorf("Must contain exactly one '=', found %v", eqCount)
		}

		tagS, value, _ := strings.Cut(field, "=")
		tag, err := strconv.Atoi(tagS)
		if err != nil {
			return nil, fmt.Errorf("Field token not an INT: %v", tagS)
		}

		result[nField] = Field{uint16(tag), value}
	}

	return result, nil
}

// Serialize to string in the Wire Format
func (msg *Message) String(sep string) string {
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
	return msg.ChecksumIgnoringFields(map[uint16]any{10: nil})
}

// Body length of the mesage, ignoring tags 8, 9 and 10
func (msg *Message) BodyLength() uint64 {
	return msg.BodyLengthIgnoringFields(map[uint16]any{8: nil, 9: nil, 10: nil})
}

// Checksum of message, can provide custom tags that needs to be ignored
func (msg *Message) ChecksumIgnoringFields(ignoreTags map[uint16]any) uint8 {
	var result int
	for _, field := range *msg {
		if _, ok := ignoreTags[field.Tag]; !ok {
			for _, ch := range field.ToWire() + "\x01" {
				result = result + int(ch)
			}
		}
	}
	return uint8(result % 256)
}

// Body length of the mesage, can provide custom tags that needs to be ignored
func (msg *Message) BodyLengthIgnoringFields(ignoreTags map[uint16]any) uint64 {
	var result uint64
	for _, field := range *msg {
		if _, ok := ignoreTags[field.Tag]; !ok {
			result += uint64(len(field.ToWire())) + 1
		}
	}
	return result
}
