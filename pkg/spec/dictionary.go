package spec

import (
	"fmt"
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
	result.Header, err = compileEntries(raw.Header.Entries, components, result.FieldNames)
	if err != nil {
		return Spec{}, err
	}

	// Flatten and validate trailer
	result.Trailer, err = compileEntries(raw.Trailer.Entries, components, result.FieldNames)
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
		switch rawEntry.XMLName.Local {
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
			return Entry{}, fmt.Errorf("Unknown XML tag entry: %v", rawEntry.XMLName.Local)
		}
	}

	// Prepare the lookup table for parent caller
	for pos, child := range result.Entries {
		tag := fields[child.Name]
		result.Lookup[tag] = pos
	}

	return result, nil
}
