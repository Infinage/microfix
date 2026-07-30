package inspector

import (
	"encoding/json"
	"maps"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

type FieldNode struct {
	Tag      uint16
	Name     string
	Value    string
	EnumDesc string
	IsGroup  bool
	Children [][]FieldNode
}

func (node *FieldNode) json(mp *map[uint16]any) {
	// Not a group - base case just add & return
	if !node.IsGroup {
		(*mp)[node.Tag] = node.Value
		return
	}

	// Is a group, recurse inside and tag will have a list
	children := make([]map[uint16]any, 0)
	for _, group := range node.Children {
		var childMp = make(map[uint16]any)
		for _, child := range group {
			child.json(&childMp)
		}
		children = append(children, childMp)
	}

	// { "268": [ {"269": "0", "270": "150.5"}, ... ] }
	(*mp)[node.Tag] = children
}

type InspectView struct {
	Name      string
	MsgId     string
	Timestamp time.Time

	Header    []FieldNode
	Body      []FieldNode
	Trailer   []FieldNode
	LeftOvers []FieldNode

	RawFix  string
	LogType string
	JSON    string

	IsValid      bool
	Observations []string
}

func (view *InspectView) json() map[uint16]any {
	var result = make(map[uint16]any)
	for _, field := range view.Header {
		field.json(&result)
	}
	for _, field := range view.Body {
		field.json(&result)
	}
	for _, field := range view.Trailer {
		field.json(&result)
	}
	for _, field := range view.LeftOvers {
		field.json(&result)
	}
	return result
}

func NewInspectView(raw, logType string, router *spec.Router, vmode spec.ValidationMode) InspectView {
	// Cleanup spaces before passing forward
	raw = strings.TrimSpace(raw)

	var result = InspectView{RawFix: raw, LogType: logType}
	if len(raw) < 4 {
		result.Observations = append(result.Observations, "Input must be atleast 4 chars long")
		return result
	}

	msg, err := message.MessageFromString(raw, raw[len(raw)-1:])
	if err != nil {
		result.Observations = append(result.Observations, err.Error())
		return result
	}

	// Extract Message ID
	msgType, _ := msg.Get(35)
	result.MsgId = msgType

	// Extract timestamp
	tsField, tsPos := msg.FindFrom(52, 0)
	if tsPos != -1 {
		ts, err := tsField.AsTZTimestamp()
		if err == nil {
			result.Timestamp = ts
		}
	}

	// Pos marks the index upto which we have parsed so far, we will proceed in phases
	var pos int
	header, trailer := router.SessionSpec().Header, router.SessionSpec().Trailer

	// Header terminates on body / trailer start
	headerTerminators := make(map[uint16]int)
	maps.Copy(headerTerminators, trailer.Lookup)
	msgEntry, msgEntryOk := router.SpecForMsgType(msgType).Messages[msgType]
	if msgEntryOk {
		maps.Copy(headerTerminators, msgEntry.Lookup)
	}

	// Create grouping for header (soft boundary - break on body / trailer tags)
	pos, result.Header = walkSpec(&msg, pos, header, headerTerminators, router.Field)

	// Create grouping for body (Soft boundary - break on trailer tags)
	if msgEntryOk {
		result.Name = msgEntry.Name
		pos, result.Body = walkSpec(&msg, pos, msgEntry, trailer.Lookup, router.Field)
	} else {
		pos, result.Body = walkSpecBasic(&msg, pos, router.Field, trailer.Lookup)
	}

	// Create the grouping for trailer (strict boundary)
	pos, result.Trailer = walkSpec(&msg, pos, trailer, nil, router.Field)

	// Collect all leftover tags, if any
	_, result.LeftOvers = walkSpecBasic(&msg, pos, router.Field, nil)

	// Build JSON string
	buf, err := json.Marshal(result.json())
	if err == nil {
		result.JSON = string(buf)
	}

	// Valiate from the router
	obs, _ := router.Validate(&msg, vmode)
	result.Observations = append(result.Observations, obs...)
	result.IsValid = len(result.Observations) == 0

	return result
}

// Does not intend to validate messages, ignores errors when it can
// Returns index at which it has stopped processing
func walkSpec(msg *message.Message, pos int, context spec.Entry, terminateOnlyOn map[uint16]int,
	fieldFn func(uint16) (spec.FieldDef, bool)) (int, []FieldNode) {
	// As we process tags, we remove them from this map to track missing required tags
	// Clone is required since delete may delete globally from spec
	localLookup := maps.Clone(context.Lookup)

	var result []FieldNode
	for pos < len(*msg) {
		field := (*msg)[pos]
		var node = FieldNode{Tag: field.Tag, Value: field.Value}

		// Exit early tag out of context
		ctxPos, inCtx := localLookup[field.Tag]
		if !inCtx {
			if _, knownGlobally := fieldFn(field.Tag); !knownGlobally {
				// Completely unknown tag. Render it as flat node and continue.
				result = append(result, node)
				pos++
				continue
			}

			// Tag is KNOWN globally, but out of context.
			// Check if it is a hard terminator for this block.
			_, isTerminal := terminateOnlyOn[field.Tag]
			if terminateOnlyOn == nil || isTerminal {
				break // Context cleanly ended
			}

			// Soft Boundary: Tag is OOC but not a terminator.
			// Populate its dictionary info, attach it to the current UI block, and continue.
			if fdef, ok := fieldFn(field.Tag); ok {
				node.Name = fdef.Name
				for _, enum := range fdef.Enums {
					if enum.Enum == node.Value {
						node.EnumDesc = enum.Description
						break
					}
				}
			}

			result = append(result, node)
			pos++
			continue
		}

		// Erase from context so that we don't consume same
		// tag multiple times when inside a group context
		delete(localLookup, field.Tag)

		// Extract name and enum desc
		fdef, ok := fieldFn(field.Tag)
		if ok {
			node.Name = fdef.Name
			for _, enum := range fdef.Enums {
				if enum.Enum == node.Value {
					node.EnumDesc = enum.Description
					break
				}
			}
		}

		// If current tag is a group, eg. "NoOrders"
		if nextContext := context.Entries[ctxPos]; nextContext.IsGroup {
			node.IsGroup = true
			if repeat, err := field.AsUint(); err == nil {
				// We ignore errors, handled by 'router.Validate'
				for range repeat {
					// Absorb unknown / OOC tags, terminate on anchor / trailer tags
					var termOnlyOnForGroup map[uint16]int
					if pos+1 < len(*msg) {
						termOnlyOnForGroup = make(map[uint16]int)
						termOnlyOnForGroup[(*msg)[pos+1].Tag] = -1
						maps.Copy(termOnlyOnForGroup, terminateOnlyOn)
						maps.Copy(termOnlyOnForGroup, localLookup)
					}
					nextPos, children := walkSpec(msg, pos+1, nextContext, termOnlyOnForGroup, fieldFn)
					node.Children = append(node.Children, children)
					pos = nextPos - 1 // Adjust to fit in with group repeat and incr below
				}
			}
		}

		result = append(result, node)
		pos++
	}

	return pos, result
}

// Runs through the message until an out of context tag is hit. Only populates the 'Tag', 'Name', 'Value', 'EnumDesc' fields.
// Returns the position at which the function has stopped. No grouping or recursion involved here
func walkSpecBasic(msg *message.Message, pos int, fieldFn func(uint16) (spec.FieldDef, bool),
	notInContext map[uint16]int) (int, []FieldNode) {

	var result []FieldNode
	for pos < len(*msg) {
		// Out of context tag hit, return
		field := (*msg)[pos]
		if _, ok := notInContext[field.Tag]; ok {
			break
		}

		// If field name and desc available populate it
		var node = FieldNode{Tag: field.Tag, Value: field.Value}
		fdef, ok := fieldFn(field.Tag)
		if ok {
			node.Name = fdef.Name
			for _, enum := range fdef.Enums {
				if enum.Enum == node.Value {
					node.EnumDesc = enum.Description
					break
				}
			}
		}

		result = append(result, node)
		pos++
	}

	return pos, result
}
