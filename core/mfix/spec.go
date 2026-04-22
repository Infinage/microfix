package mfix

import (
	"fmt"
	"slices"
	"strconv"
)

type Entry struct {
	Name     string // Field name
	Required bool   // If required per spec

	// Below fields only meaningful for groups
	IsGroup bool           // If true, expect to have entries
	Entries []Entry        // Ordered list of nested fields
	Lookup  map[uint16]int // Quicker lookups to point to position on Entries
}

// Cleaned struct for quicker lookups
type Spec struct {
	Major      int
	Minor      int
	SP         int
	Header     Entry
	Trailer    Entry
	Messages   map[string]Entry    // lookup by msgtype
	Fields     map[uint16]FieldDef // Lookup by tag ('Number')
	FieldNames map[string]uint16   // Lookup tag by field name
}

type componentContext struct {
	unflattened []specEntry
	flattened   Entry

	// 0 - Not visited
	// 1 - Currently visiting
	// 2 - Flattened
	state uint8
}

// Load a spec from given file path
func LoadSpec(path string) (Spec, error) {
	raw, err := loadRawSpec(path)
	if err != nil {
		return Spec{}, err
	}

	// Convert rawSpec to spec for faster lookups
	var result = Spec{
		Major:      raw.Major,
		Minor:      raw.Minor,
		SP:         raw.Sp,
		Messages:   make(map[string]Entry),
		Fields:     make(map[uint16]FieldDef),
		FieldNames: make(map[string]uint16),
	}

	// Fields slice into map for quicker lookup
	for _, field := range raw.Fields {
		tag := uint16(field.Number)
		result.Fields[tag] = field
		result.FieldNames[field.Name] = tag
	}

	// Load components into temp map for quicker lookup
	var components = make(map[string]*componentContext)
	for _, comp := range raw.Components {
		components[comp.Name] = &componentContext{unflattened: comp.Entries}
	}

	// Flatten and validate header
	result.Header, err = compileEntries(raw.Header, components, result.FieldNames)
	if err != nil {
		return Spec{}, err
	}

	// Flatten and validate trailer
	result.Trailer, err = compileEntries(raw.Trailer, components, result.FieldNames)
	if err != nil {
		return Spec{}, err
	}

	// Iterate through message entries while flattening + validating
	for _, message := range raw.Messages {
		flattenedMsg, err := compileEntries(message.Entries, components, result.FieldNames)
		if err != nil {
			return Spec{}, err
		}
		result.Messages[message.MsgType] = flattenedMsg
	}

	return result, nil
}

// Recursively validate fields and flatten the []specEntry removing need for component map
// To save some compute, we cache flattened component results
// We write results to a more friendly struct - Entry
func compileEntries(message []specEntry, components map[string]*componentContext,
	fields map[string]uint16) (Entry, error) {

	var result = Entry{Lookup: make(map[uint16]int)}
	for _, rawEntry := range message {
		switch rawEntry.Type.Local {
		case "field":
			if _, ok := fields[rawEntry.Name]; !ok {
				return Entry{}, fmt.Errorf("Field name not found: %v", rawEntry.Name)
			}
			result.Entries = append(result.Entries, Entry{Name: rawEntry.Name, Required: bool(rawEntry.Required)})

		case "component":
			component, found := components[rawEntry.Name]
			if !found {
				return Entry{}, fmt.Errorf("Component name not found: %v", rawEntry.Name)
			}

			// Already being processed
			if component.state == 1 {
				return Entry{}, fmt.Errorf("Circular component reference detected: %v", rawEntry.Name)
			}

			// Not flattened yet
			if component.state == 0 {
				component.state = 1
				flattenedEntries, err := compileEntries(component.unflattened, components, fields)
				if err != nil {
					return Entry{}, err
				}

				// Cache the updated entries
				component.flattened = flattenedEntries
				component.state = 2
			}

			// Add the flattened entries into our result vector directly
			// Remove the outer layer / node. The final output will be as if
			// the component never existed to begin with
			for _, compEntry := range component.flattened.Entries {
				compEntry.Required = compEntry.Required && bool(rawEntry.Required)
				result.Entries = append(result.Entries, compEntry)
			}

		case "group":
			// Ensure that No<NAME> is present in fields
			if _, ok := fields[rawEntry.Name]; !ok {
				return Entry{}, fmt.Errorf("Group name %v not found in fields", rawEntry.Name)
			}

			// Recursively validate of entries of groups are ok, also
			// unflatten any component that it may contain
			groupEntry, err := compileEntries(rawEntry.Entries, components, fields)
			if err != nil {
				return Entry{}, err
			}

			// Add the updated group entries while retaining the
			// outer node to mark that current entry is a group
			// Fill in the required meta before adding it
			groupEntry.Name = rawEntry.Name
			groupEntry.IsGroup = true
			groupEntry.Required = bool(rawEntry.Required)
			for pos, child := range groupEntry.Entries {
				tag := fields[child.Name]
				groupEntry.Lookup[tag] = pos
			}
			result.Entries = append(result.Entries, groupEntry)

		default:
			return Entry{}, fmt.Errorf("Unknown XML tag entry: %v", rawEntry.Type.Local)
		}
	}

	// Prepare the lookup table for parent caller
	for pos, child := range result.Entries {
		tag := fields[child.Name]
		result.Lookup[tag] = pos
	}

	return result, nil
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
	groupCountOverides map[uint16]int) (Message, error) {

	// Helper to recurse entry one at a time, if group we recurse into it otherwise just add it
	var addEntry func(msg *Message, entry Entry, requiredOnly bool, groupCountOverides *map[uint16]int) error
	addEntry = func(msg *Message, entry Entry, requiredOnly bool, groupCountOverides *map[uint16]int) error {
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
			*msg = append(*msg, Field{tag, value})
		} else {
			tag, _ := spec.FieldNames[entry.Name]
			var repeat int
			if count, ok := (*groupCountOverides)[tag]; ok {
				repeat = count
			}
			*msg = append(*msg, Field{tag, strconv.Itoa(repeat)})
			for range repeat {
				for _, subEntry := range entry.Entries {
					addEntry(msg, subEntry, requiredOnly, groupCountOverides)
				}
			}
		}

		return nil
	}

	var result Message
	msgSpec, ok := spec.Messages[msgType]
	if !ok {
		return result, fmt.Errorf("MsgType [%v] not found in spec", msgType)
	}

	for _, entry := range slices.Concat(spec.Header.Entries, msgSpec.Entries, spec.Trailer.Entries) {
		err := addEntry(&result, entry, requiredOnly, &groupCountOverides)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

// Constants for Validate function
type ValidationMode int

const (
	None   ValidationMode = iota // no validation
	Basic                        // checksum, bodylen, required fields, groups
	Strict                       // type check, unknown fields check
)

// Validate an input message and return list of observations
func (spec *Spec) Validate(message *Message, mode ValidationMode) (bool, []string) {
	var observations []string
	if mode == None {
		return true, observations
	}

	msgType, pos := message.Find(35)
	if pos == -1 {
		observations = append(observations, fmt.Sprint("MsgType Tag (35) missing"))
		return false, observations
	}

	// Checksum validation if required
	if _, ok := spec.Trailer.Lookup[10]; ok {
		checksumTag, pos := message.Find(10)
		if pos == -1 {
			observations = append(observations, fmt.Sprint("Missing checksum tag [10]"))
		} else if want := fmt.Sprintf("%03d", Checksum(message)); want != checksumTag.Value {
			observations = append(observations, fmt.Sprintf("Checksum validation failed: want %v, got %v",
				want, checksumTag.Value))
		}
	}

	// Bodylength validation if required
	if _, ok := spec.Header.Lookup[9]; ok {
		bodylength := BodyLength(message)
		bodyLenTag, pos := message.Find(9)
		if pos == -1 {
			observations = append(observations, fmt.Sprint("Missing bodylength tag [9]"))
		} else if got, err := bodyLenTag.AsUint(); err != nil || bodylength != got {
			observations = append(observations, fmt.Sprint("Bodylength validation failed: want %v, got %v",
				bodylength, got))
		}
	}

	msgSpec, ok := spec.Messages[msgType.Value]
	if !ok {
		observations = append(observations, fmt.Sprintf("Unknown MsgType '35=%v'", msgType.Value))
		return false, observations
	}

	// Walk through and validate for entries against header, msg body and trailer
	var err error
	pos, err = walkSpec(message, spec.Header, 0, observations)
	if err != nil {
		return false, observations
	}
	pos, err = walkSpec(message, msgSpec, pos, observations)
	if err != nil {
		return false, observations
	}
	pos, err = walkSpec(message, spec.Trailer, pos, observations)
	if err != nil {
		return false, observations
	}

	// Validate for types and unknown fields
	if mode == Strict {
		for _, field := range *message {
			var found bool
			if _, ok := spec.Header.Lookup[field.Tag]; ok {
				found = true
			}
			if _, ok := msgSpec.Lookup[field.Tag]; ok {
				found = true
			}
			if _, ok := spec.Trailer.Lookup[field.Tag]; ok {
				found = true
			}

			// Validate the data type
			if found {
				if err := validateDtype(field, spec.Fields[field.Tag].Type); err != nil {
					observations = append(observations, fmt.Sprintf("Data validation failed for %v", field.Tag))	
				}
			} else {
				observations = append(observations, fmt.Sprintf("Unknown tag %v", field.Tag))
			}
		}	
	}

	if pos != len(*message) {
		observations = append(observations, fmt.Sprintf("Message entry #%v didn't match the spec", pos))
	}

	return len(observations) == 0, observations
}

// Returns index just after processing the message for that context
func walkSpec(msg *Message, context Entry, idx int, obs []string) (int, error) {
	for idx < len(*msg) {
		// Get the field and look it up from spec
		field := (*msg)[idx]
		pos, exists := context.Lookup[field.Tag]
		if !exists {
			break
		}

		// Get a copy of the entry from spec and mark as visited
		entry := context.Entries[pos]
		delete(context.Lookup, field.Tag)

		// If group, recurse into it specified no of times
		if entry.IsGroup {
			repeat, err := field.AsUint()
			if err != nil {
				return idx, fmt.Errorf("Expected group tag to have integer value, got %v", field.Value)
			}

			for range repeat {
				// Ensure first tag in group is our anchor tag from spec
				if anchorPos, found := entry.Lookup[(*msg)[idx + 1].Tag]; !found || anchorPos != 0 {
					obs = append(obs, fmt.Sprintf("Tag %v immediately following group missing" + 
						" or not at first position on Group Spec", (*msg)[idx + 1].Tag))
				}

				// Recurse for that repeating group
				idx, err := walkSpec(msg, entry, idx + 1, obs)
				if err != nil {
					return idx, err
				}
			}

			// Walk spec already updated idx to point just after current scope
			continue
		}

		idx++
	}

	// Check for required tags still pending processing
	for tag, pos := range context.Lookup {
		if context.Entries[pos].Required {
			obs = append(obs, fmt.Sprintf("Missing required field tag [%v]", tag))
		}
	}

	// Fail the check if any observations in current context
	if len(obs) > 0 {
		return idx, fmt.Errorf("Observed %v issues processing message", len(obs))
	}

	return idx, nil
}
