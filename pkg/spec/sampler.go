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

// Configuration for generating a template message
type SampleOptions struct {
	// IncludeOptional toggles the generation of non-required fields
	// If OptionalFields is not nil, this is assumed to be set as true
	IncludeOptional bool

	// OptionalFields, if not nil, acts as a whitelist of tags to include
	// when IncludeOptional is true
	OptionalFields map[uint16]any

	// GroupOverrides specifies how many times a repeating group should
	// iterate (keyed by the group's NumInGroup tag)
	GroupOverrides map[uint16]int
}

// Sample generates a message template based on the Spec.
// It handles required fields, whitelisted optional fields (even deep in components),
func (spec *Spec) Sample(msgType string, opts SampleOptions) (message.Message, error) {

	// hasWhitelistedDescendant checks if the current entry or any of its
	// children/groups contain a tag present in the whitelist.
	var hasWhitelistedDescendant func(e *Entry) bool
	hasWhitelistedDescendant = func(e *Entry) bool {
		tag, _ := spec.FieldNames[e.Name]
		if _, ok := opts.OptionalFields[tag]; ok {
			return true
		}
		return slices.ContainsFunc(e.Entries, func(child Entry) bool {
			return hasWhitelistedDescendant(&child)
		})
	}

	var addEntry func(msg *message.Message, entry Entry) error
	addEntry = func(msg *message.Message, entry Entry) error {
		tag, _ := spec.FieldNames[entry.Name]

		// Filtering Logic:
		// 1. Required fields always pass.
		// 2. If a whitelist is provided, pass if this tag OR any child is whitelisted.
		// 3. Otherwise, pass only if IncludeOptional is toggled on.
		if !entry.Required {
			if opts.OptionalFields != nil {
				if !hasWhitelistedDescendant(&entry) {
					return nil
				}
			} else if !opts.IncludeOptional {
				return nil
			}
		}

		if !entry.IsGroup {
			field, _ := spec.Fields[tag]
			var value string = defaultString(field.Type)
			if len(field.Enums) > 0 {
				value = field.Enums[0].Enum
			}
			*msg = append(*msg, message.Field{Tag: tag, Value: value})
		} else {
			var repeat = 1
			if count, ok := opts.GroupOverrides[tag]; ok {
				repeat = count
			}

			// Add the NumInGroup counter tag
			*msg = append(*msg, message.Field{Tag: tag, Value: strconv.Itoa(repeat)})

			// Recurse into group members
			for range repeat {
				for _, subEntry := range entry.Entries {
					addEntry(msg, subEntry)
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

	// Assemble Header + Body + Trailer
	layout := slices.Concat(spec.Header.Entries, msgSpec.Entries, spec.Trailer.Entries)
	for _, entry := range layout {
		if err := addEntry(&result, entry); err != nil {
			return result, err
		}
	}

	// Update MsgType or insert if missing (ideally should never be missing)
	if !result.Set(35, msgType) {
		result.Insert(min(2, len(result)), message.Field{Tag: 35, Value: msgType})
	}

	// Update BeginString or insert if missing (ideally should never be missing)
	beginString := fmt.Sprintf("%v.%d.%d", spec.Type, spec.Major, spec.Minor)
	if !result.Set(8, beginString) {
		result.Insert(0, message.Field{Tag: 8, Value: beginString})
	}

	// BodyLength and Checksum calculation
	result.Finalize()
	return result, nil
}
