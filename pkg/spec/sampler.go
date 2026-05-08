package spec

import (
	"fmt"
	"slices"
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

// Internal helper that takes in a list of entries and populates them into message struct
func (spec *Spec) buildFromEntries(entries []Entry, opts SampleOptions) message.Message {

	// This helper recursively checks if the current entry or any of
	// its children/groups contain a tag present in the whitelist.
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

	// Recursively process an entry and add it fields into the message being built
	var addEntry func(msg *message.Message, entry Entry)
	addEntry = func(msg *message.Message, entry Entry) {
		tag, _ := spec.FieldNames[entry.Name]

		// Shortcut to skip adding entry's contents into message.
		// 1. Required fields are never skipped
		// 2. If a whitelist is provided, pass if this tag OR any child is whitelisted.
		// 3. Otherwise, pass only if IncludeOptional is toggled on.
		if !entry.Required {
			if opts.OptionalFields != nil {
				if !hasWhitelistedDescendant(&entry) {
					return
				}
			} else if !opts.IncludeOptional {
				return
			}
		}

		// If field, try to check if it contains enum entries
		// Prefer picking first enum over defaulted values
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
			*msg = append(*msg, message.Field{Tag: tag, Value: fmt.Sprint(repeat)})

			// Recurse into group members
			for range repeat {
				for _, subEntry := range entry.Entries {
					addEntry(msg, subEntry)
				}
			}
		}
	}

	var result message.Message
	for _, entry := range entries {
		addEntry(&result, entry)
	}

	return result
}

// Sample just the header from spec
func (spec *Spec) SampleHeader(opts SampleOptions) message.Message {
	return spec.buildFromEntries(spec.Header.Entries, opts)
}

// Sample just the trailer from spec
func (spec *Spec) SampleTrailer(opts SampleOptions) message.Message {
	return spec.buildFromEntries(spec.Trailer.Entries, opts)
}

// Sample body from spec given MsgType, if MsgType is missing returns an error
func (spec *Spec) SampleBody(msgType string, opts SampleOptions) (message.Message, error) {
	msgSpec, ok := spec.Messages[msgType]
	if !ok {
		return message.Message{}, fmt.Errorf("MsgType [%v] not found in spec", msgType)
	}

	return spec.buildFromEntries(msgSpec.Entries, opts), nil
}

// IsAdmin checks if the message type is a session level message
func (r *Router) IsAdmin(msgType string) bool {
	switch msgType {
	case "0", "1", "2", "3", "4", "5", "A", "n":
		return true
	}
	return false
}

// Build a sample message from spec, headers/trailer are picked from session
// spec while the body is picked from the applSpec that currenlty selected
// ---
// Ensure that Session/Engine sets correct values for required tags
// and Finalize is called once again before send
func (r *Router) Sample(msgType string, opts SampleOptions) (message.Message, error) {
	// Route the message correctly to session layer or appl layer
	var msgSpec *Spec = r.DefaultApplSpec()
	if r.IsAdmin(msgType) {
		msgSpec = r.DefaultSessionSpec()
	}

	// Returns err if MsgType is not found
	body, err := msgSpec.SampleBody(msgType, opts)
	if err != nil {
		return message.Message{}, err
	}

	// Sample the header and body from the session layer spec
	header := r.DefaultSessionSpec().SampleHeader(opts)
	trailer := r.DefaultSessionSpec().SampleTrailer(opts)

	// Construct the message from constituents
	var result message.Message
	result = append(result, header...)
	result = append(result, body...)
	result = append(result, trailer...)

	// Inject context into message
	result.Set(8, r.DefaultSessionSpec().BeginString())
	result.Set(35, msgType)

	// Calculate bodylen and checksum
	result.Finalize()

	return result, nil
}
