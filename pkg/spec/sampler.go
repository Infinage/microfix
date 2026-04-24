package spec

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/infinage/microfix/pkg/message"
)

// Default string given dtype from xml spec
func defaultString(dtype string) string {
	switch strings.ToLower(dtype) {
	case "int", "seqnum", "tagnum", "length", "numingroup":
		return "4"

	case "amt", "float", "percentage", "price", "priceoffset", "qty":
		return "7.0466"

	case "boolean":
		return "N"

	case "char":
		return "J"

	case "multiplecharvalue":
		return "A B"

	case "multiplestringvalue", "multiplevaluestring":
		return "AB CD"

	case "utcdateonly", "localmktdate", "date":
		return "19660407"

	case "utctimeonly", "localmkttime", "time":
		return "12:00:00"

	case "utctimestamp", "utcdate", "tztimestamp":
		return "20260404-12:00:00Z"

	case "tztimeonly":
		return "12:00:00Z"

	case "monthyear":
		return "202604"

	default:
		return dtype
	}
}

func (spec *Spec) Field(tag uint16) (FieldDef, error) {
	res, ok := spec.Fields[tag]
	if !ok {
		return FieldDef{}, fmt.Errorf("Tag [%v] not found in the spec", tag)
	}

	return res, nil
}

// Sample a message type
func (spec *Spec) Sample(msgType string, requiredOnly bool,
	groupCountOverides map[uint16]int) (message.Message, error) {

	// Helper to recurse entry one at a time, if group we recurse into it otherwise just add it
	var addEntry func(msg *message.Message, entry Entry, requiredOnly bool, groupCountOverides *map[uint16]int) error
	addEntry = func(msg *message.Message, entry Entry, requiredOnly bool, groupCountOverides *map[uint16]int) error {
		if requiredOnly && !entry.Required {
			return nil
		}

		if !entry.IsGroup {
			tag, _ := spec.FieldNames[entry.Name]
			field, _ := spec.Fields[tag]
			var value string = defaultString(field.Type)
			if len(field.Enums) > 0 {
				value = field.Enums[0].Enum
			}
			*msg = append(*msg, message.Field{Tag: tag, Value: value})
		} else {
			tag, _ := spec.FieldNames[entry.Name]
			var repeat int = 1
			if count, ok := (*groupCountOverides)[tag]; ok {
				repeat = count
			}
			*msg = append(*msg, message.Field{Tag: tag, Value: strconv.Itoa(repeat)})
			for range repeat {
				for _, subEntry := range entry.Entries {
					addEntry(msg, subEntry, requiredOnly, groupCountOverides)
				}
			}
		}

		return nil
	}

	var result message.Message
	msgSpec, ok := spec.Messages[msgType]
	if !ok {
		return result, fmt.Errorf("MsgType [%v] not found in spec", msgType)
	}

	// Add the message body and trailer
	for _, entry := range slices.Concat(spec.Header.Entries, msgSpec.Entries, spec.Trailer.Entries) {
		err := addEntry(&result, entry, requiredOnly, &groupCountOverides)
		if err != nil {
			return result, err
		}
	}

	// Add msgtype, bodylen and checksum if required
	spec.Finalize(&result, msgType)

	return result, nil
}

// Add MessageType, Checksum and Bodylength if missing or update it
func (spec *Spec) Finalize(msg *message.Message, msgTypeVal string) {
	if _, required := spec.Header.Lookup[9]; required {
		if bodyLen := fmt.Sprint(msg.BodyLength()); !msg.Set(9, bodyLen) {
			field := message.Field{Tag: 9, Value: bodyLen}
			msg.Insert(1, field)
		}
	}

	if _, required := spec.Header.Lookup[35]; required {
		if !msg.Set(35, msgTypeVal) {
			field := message.Field{Tag: 35, Value: msgTypeVal}
			msg.Insert(min(2, len(*msg)), field)
		}
	}

	if _, required := spec.Trailer.Lookup[10]; required {
		if checksum := fmt.Sprintf("%03d", msg.Checksum()); !msg.Set(10, checksum) {
			field := message.Field{Tag: 10, Value: checksum}
			msg.Insert(len(*msg)-1, field)
		}
	}
}
