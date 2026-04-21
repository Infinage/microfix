package mfix

import (
	"embed"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// BoolYN handles the FIX 'Y'/'N' attribute mapping
type BoolYN bool

func (b *BoolYN) UnmarshalXMLAttr(attr xml.Attr) error {
	*b = BoolYN(attr.Value == "Y")
	return nil
}

// Entry represents any item inside a message, component or group.
type Entry struct {
	Type     xml.Name
	Name     string  `xml:"name,attr"`
	Required BoolYN  `xml:"required,attr"`
	Entries  []Entry `xml:",any"`
}

// Struct for `<messages><message/></messages>`
type messageDef struct {
	Name    string  `xml:"name,attr"`
	MsgType string  `xml:"msgtype,attr"`
	Entries []Entry `xml:",any"`
}

// Struct for `<components><component/><components>`
type componentDef struct {
	Name      string  `xml:"name,attr"`
	Entries   []Entry `xml:",any"`
	flattened bool
	visiting  bool
}

// Struct for `<fields><field/><field>`
type FieldDef struct {
	Number int    `xml:"number,attr"`
	Name   string `xml:"name,attr"`
	Type   string `xml:"type,attr"`
	Enums  []struct {
		Enum        string `xml:"enum,attr"`
		Description string `xml:"description,attr"`
	} `xml:"value"`
}

// RawSpec matches spec.xml exactly
type rawSpec struct {
	Name       xml.Name       `xml:"fix"`
	Major      int            `xml:"major,attr"`
	Minor      int            `xml:"minor,attr"`
	Sp         int            `xml:"servicepack,attr"`
	Header     []Entry        `xml:"header>*"`
	Messages   []messageDef   `xml:"messages>message"`
	Trailer    []Entry        `xml:"trailer>*"`
	Components []componentDef `xml:"components>component"`
	Fields     []FieldDef     `xml:"fields>field"`
}

// Cleaned struct for quicker lookups
type Spec struct {
	Major      int
	Minor      int
	SP         int
	Header     []Entry
	Trailer    []Entry
	Messages   map[string][]Entry  // lookup by msgtype
	Fields     map[uint16]FieldDef // Lookup by tag ('Number')
	FieldNames map[string]uint16   // Lookup tag by field name
}

//go:embed spec/*.xml
var defaultSpecs embed.FS

// Load a spec from given file path
func LoadSpec(path string) (Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		raw, err = defaultSpecs.ReadFile(filepath.Join("spec", path))
		if err != nil {
			return Spec{}, fmt.Errorf("Could not find spec %s in local or embedded path", path)
		}
	}

	// Load into temp raw struct
	var data rawSpec
	err = xml.Unmarshal(raw, &data)
	if err != nil {
		return Spec{}, err
	}

	// Convert rawSpec to spec for faster lookups
	var result = Spec{
		Major:      data.Major,
		Minor:      data.Minor,
		SP:         data.Sp,
		Messages:   make(map[string][]Entry),
		Fields:     make(map[uint16]FieldDef),
		FieldNames: make(map[string]uint16),
	}

	// Fields slice into map for quicker lookup
	for _, field := range data.Fields {
		tag := uint16(field.Number)
		result.Fields[tag] = field
		result.FieldNames[field.Name] = tag
	}

	// Load components into temp map for quicker lookup
	var components = make(map[string]*componentDef)
	for i := range data.Components {
		temp := data.Components[i]
		components[temp.Name] = &temp
	}

	// Flatten and validate header
	result.Header, err = vflatten(data.Header, components, result.FieldNames)
	if err != nil {
		return Spec{}, err
	}

	// Flatten and validate trailer
	result.Trailer, err = vflatten(data.Trailer, components, result.FieldNames)
	if err != nil {
		return Spec{}, err
	}

	// Iterate through message entries while flattening + validating
	for _, message := range data.Messages {
		flattenedMsg, err := vflatten(message.Entries, components, result.FieldNames)
		if err != nil {
			return Spec{}, err
		}
		result.Messages[message.MsgType] = flattenedMsg
	}

	return result, nil
}

// Recursively validate fields and flatten the []Entry removing need for component map
// To save some compute, we cache flattened component results
// Final result's entry.Type.Local will be either a field or a group
func vflatten(message []Entry, components map[string]*componentDef,
	fields map[string]uint16) ([]Entry, error) {

	var flattened []Entry
	for _, entry := range message {
		switch entry.Type.Local {
		case "field":
			if _, ok := fields[entry.Name]; !ok {
				return nil, fmt.Errorf("Field name not found: %v", entry.Name)
			}
			flattened = append(flattened, entry)

		case "component":
			component, found := components[entry.Name]
			if !found {
				return nil, fmt.Errorf("Component name not found: %v", entry.Name)
			}

			if component.visiting {
				return nil, fmt.Errorf("Circular component reference detected: %v", entry.Name)
			}

			if !component.flattened {
				component.visiting = true
				flattenedEntries, err := vflatten(component.Entries, components, fields)
				if err != nil {
					return nil, err
				}

				// Cache the updated entries
				component.Entries = flattenedEntries
				component.visiting = false
				component.flattened = true
			}

			// Add the flattened entries into our result vector directly
			// Remove the outer layer / node. The final output will be as if
			// the component never existed to begin with
			for _, compEntry := range component.Entries {
				compEntry.Required = compEntry.Required && entry.Required
				flattened = append(flattened, compEntry)
			}

		case "group":
			// Ensure that No<NAME> is present in fields
			if _, ok := fields[entry.Name]; !ok {
				return nil, fmt.Errorf("Group name %v not found in fields", entry.Name)
			}

			nextEntries, err := vflatten(entry.Entries, components, fields)
			if err != nil {
				return nil, err
			}

			// Added the updated group entries while retaining the
			// outer node to mark that current entry is a group
			entry.Entries = nextEntries
			flattened = append(flattened, entry)

		default:
			return nil, fmt.Errorf("Unknown XML tag entry: %v", entry.Type.Local)
		}
	}

	return flattened, nil
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
		if requiredOnly && !bool(entry.Required) {
			return nil
		}

		switch entry.Type.Local {
		case "field":
			tag, _ := spec.FieldNames[entry.Name]
			field, _ := spec.Fields[tag]
			var value string = field.Type
			if len(field.Enums) > 0 {
				value = field.Enums[0].Enum
			}
			*msg = append(*msg, Field{tag, value})

		case "group":
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

		default:
			return fmt.Errorf("Unexpected XML node of type %v", entry.Type.Local)
		}

		return nil
	}

	var result Message
	msgSpec, ok := spec.Messages[msgType]
	if !ok {
		return result, fmt.Errorf("MsgType [%v] not found in spec", msgType)
	}

	for _, entry := range msgSpec {
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

	msgSpec, ok := spec.Messages[msgType.value]
	if !ok {
		observations = append(observations, fmt.Sprintf("Unknown MsgType '35=%v'", msgType.value))
		return false, observations
	}

	// Set to collect all requried and optional tags
	var requiredTags = make(map[uint16]interface{})
	var optionalTags = make(map[uint16]interface{})

	// Helper to iterate across entries and collect them
	var collectTags = func(entries []Entry, required map[uint16]interface{}, optional map[uint16]interface{}) {
		for _, entry := range entries {
			tag := spec.FieldNames[entry.Name]
			if bool(entry.Required) {
				required[tag] = nil
			} else {
				optional[tag] = nil
			}
		}
	}

	// Collect all required + opt tags into a map
	collectTags(spec.Header, requiredTags, optionalTags)
	collectTags(msgSpec, requiredTags, optionalTags)
	collectTags(spec.Trailer, requiredTags, optionalTags)

	// Checksum validation if required
	_, checksumRequired := requiredTags[10]
	if checksumRequired {
		checksumTag, pos := message.Find(10)
		if pos == -1 {
			observations = append(observations, fmt.Sprint("Missing checksum tag [10]"))
		} else if want := fmt.Sprintf("%03d", Checksum(message)); want != checksumTag.value {
			observations = append(observations, fmt.Sprint("Checksum validation failed: want %v, got %v", 
				want, checksumTag.value))
		}
	}

	// Bodylength validation if required
	_, bodylenRequired := requiredTags[9]
	if bodylenRequired {
		bodylength := BodyLength(message)
		bodyLenTag, pos := message.Find(9)
		if pos == -1 {
			observations = append(observations, fmt.Sprint("Missing bodylength tag [9]"))
		} else if got, err := bodyLenTag.AsUint(); err != nil || bodylength != got {
			observations = append(observations, fmt.Sprint("Bodylength validation failed: want %v, got %v", 
				bodylength, got))
		}
	}

	// Ensure all required fields are present
	for _, field := range *message {
		
	}

	return len(observations) == 0, observations
}
