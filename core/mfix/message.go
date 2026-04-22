package mfix

import (
	"errors"
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

// Serialize to string
func (msg Message) String(sep string) string {
	var res []string
	for _, field := range msg {
		res = append(res, field.string())
	}
	return strings.Join(res, sep) + sep
}

// Returns Field if tag is found along with its index
// index of -1 is returned if not found
func (msg Message) Find(tag uint16) (Field, int) {
	return msg.FindFrom(tag, 0)
}

// Same as Find, but can provide the starting pos to being search
func (msg Message) FindFrom(tag uint16, start int) (Field, int) {
	for i := start; i < len(msg); i++ {
		if msg[i].Tag == tag {
			return msg[i], i
		}
	}

	return Field{}, -1
}

// Iterate and return all fields matching tag
func (msg Message) FindAll(tag uint16) iter.Seq[Field] {
	return func(yield func(Field) bool) {
		for _, field := range msg {
			if field.Tag == tag {
				if !yield(field) {
					break
				}
			}
		}
	}
}

// Convenience func to return msgtype tag
func (msg Message) Code() (string, error) {
	field, idx := msg.Find(35)
	if idx == -1 {
		return "", errors.New("Tag MsgType (35) not found")
	}
	return field.Value, nil
}
