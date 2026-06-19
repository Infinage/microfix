package spec

import (
	"fmt"
	"maps"
	"strings"

	"github.com/infinage/microfix/pkg/message"
)

// Ensure input string is as per dtype from input string
func validateDtype(field message.Field, dtype string) error {
	var err error
	switch strings.ToLower(dtype) {
	case "int", "seqnum", "tagnum", "length", "numingroup":
		_, err = field.AsInt()

	case "amt", "float", "percentage", "price", "priceoffset", "qty":
		_, err = field.AsDouble()

	case "boolean":
		_, err = field.AsBool()

	case "char":
		_, err = field.AsChar()

	case "multiplecharvalue":
		_, err = field.AsCharVector()

	case "multiplestringvalue", "multiplevaluestring":
		_, err = field.AsStringVector()

	case "utcdateonly", "localmktdate", "date":
		_, err = field.AsDate()

	case "utctimeonly", "localmkttime", "time":
		_, err = field.AsTime()

	case "utctimestamp", "utcdate", "tztimestamp":
		_, err = field.AsTZTimestamp()

	case "tztimeonly":
		_, err = field.AsTZTime()

	case "monthyear":
		_, err = field.AsMonthYear()
	}
	return err
}

// Constants for Validate function
type ValidationMode int

const (
	ValidationNone   ValidationMode = iota // no validation
	ValidationBasic                        // checksum, bodylen, required fields, groups
	ValidationStrict                       // type check, unknown fields check
)

// walkSpec validates a FIX message against its specification at a specific hierarchical level (context).
// It returns the index of the next unprocessed field, or an error ONLY if structural parsing fails.
func walkSpec(ro *Router, msg *message.Message, vmode ValidationMode, context Entry,
	terminateOnlyOn map[uint16]int, idx int, obs *[]string) (int, error) {

	// localLookup tracks expected tags in the current context.
	// As we process tags, we remove them from this map to track missing required tags.
	localLookup := maps.Clone(context.Lookup)

	// Tracks if a group or context was prematurely terminated by an unexpected tag
	oocTagIdx := -1

	for idx < len(*msg) {
		field := (*msg)[idx]
		pos, exists := localLookup[field.Tag]

		// --- Context Boundary & Unknown Tag Handling ---
		if !exists {
			if _, knownField := ro.Field(field.Tag); !knownField {
				// Tag is UNKNOWN to the global dictionary.
				if vmode == ValidationStrict {
					*obs = append(*obs, fmt.Sprintf("Unknown tag [%v]", field.Tag))
				}
				idx++
				continue
			}

			// Tag is KNOWN to the global dictionary, but doesn't belong in this context.
			// If terminateOnlyOn is nil, we are in "strict boundary" mode: ANY unknown tag breaks the context.
			// If terminateOnlyOn is provided, we only break if the tag is explicitly in that map.
			_, isTerminal := terminateOnlyOn[field.Tag]
			if terminateOnlyOn == nil || isTerminal {
				oocTagIdx = idx
				break // Context cleanly ended
			}

			// Soft Boundary: The tag is out-of-context, but it's not a terminator.
			// Log it as an observation and continue validating the rest of the block.
			if vmode == ValidationStrict {
				*obs = append(*obs, fmt.Sprintf("Unexpected out-of-context tag [%v]", field.Tag))
			}
			idx++
			continue
		}

		// --- Process Valid Field ---
		entry := context.Entries[pos]
		delete(localLookup, field.Tag) // Marking as visited

		// Validate data type
		if vmode == ValidationStrict {
			fDef, _ := ro.Field(field.Tag)
			if err := validateDtype(field, fDef.Type); err != nil {
				*obs = append(*obs, fmt.Sprintf("Datatype validation failed for tag [%v]", field.Tag))
			}
		}

		// --- Handle Repeating Groups ---
		if entry.IsGroup {
			repeat, err := field.AsUint()
			if err != nil {
				// STRUCTURAL error: we cannot parse a group if the count isn't an integer
				err = fmt.Errorf("Expected group tag [%v] to have integer value, got '%v'", field.Tag, field.Value)
				*obs = append(*obs, err.Error())
				return idx, err
			}

			// Preserve the group order across repeating groups
			idx++ // We have 'processed' the group tag now
			var group1Start, groupSize = idx, -1

			for gi := range repeat {
				// Recurse for that repeating group.
				// Groups always use strict boundaries (terminateOnlyOn = nil)
				idx, err = walkSpec(ro, msg, vmode, entry, nil, idx, obs)
				if err != nil {
					return idx, err // Bubble up structural failures
				}

				// For the first grp repetition, establish the blueprint (size and anchor tag)
				if groupSize == -1 {
					// Store the begin and end indices of a group
					groupSize = idx - group1Start

					// Ensure first tag in group is our anchor tag from spec
					anchorTag := (*msg)[group1Start].Tag
					if anchorPos, found := entry.Lookup[anchorTag]; !found || anchorPos != 0 {
						*obs = append(*obs, fmt.Sprintf("Tag %v immediately following group count missing"+
							" or not at first position", (*msg)[idx+1].Tag))
					}
				} else if vmode == ValidationStrict {
					// Validate the ordering for second repeating group onwards
					groupStart := group1Start + (int(gi) * groupSize)
					for i := range groupSize {
						g0, g := (*msg)[group1Start+i], (*msg)[groupStart+i]
						if g0.Tag != g.Tag {
							*obs = append(*obs, fmt.Sprintf("Expected group #%v entry #%v to be "+
								"tag [%v], had [%v]", gi+1, i+1, g0.Tag, g.Tag))
						}
					}
				}

			}
			continue // walkSpec already updated idx
		}

		idx++
	}

	// --- Post-Processing Checks (Missing Required Tags) ---
	for tag, pos := range localLookup {
		if context.Entries[pos].Required {
			// If a required tag is missing, AND we broke out early due to an out-of-context tag,
			// it is highly likely the out-of-context tag prematurely terminated the group.
			if oocTagIdx != -1 {
				*obs = append(*obs, fmt.Sprintf("Context prematurely terminated by unexpected tag [%v]", (*msg)[oocTagIdx].Tag))
				oocTagIdx = -1 // Reset so we only log the terminator warning once
			}
			*obs = append(*obs, fmt.Sprintf("Missing required field tag [%v]", tag))
		}
	}

	// Validation warnings do not halt the parsing process
	return idx, nil
}

// Validate an input message and return list of observations
func (router *Router) Validate(msg *message.Message, mode ValidationMode) ([]string, bool) {
	var observations []string
	if mode == ValidationNone {
		return observations, true
	}

	// Check all mandatory tags by position
	// If position is -1, ignore position check
	mandatoryTags := []struct {
		t uint16
		p int
	}{
		{8, 0},              // BeginString
		{9, 1},              // BodyLength
		{35, 2},             // MsgType
		{49, -1},            // SenderCompID
		{56, -1},            // TargetCompID
		{34, -1},            // MsgSeqNum
		{52, -1},            // SendingTime
		{10, len(*msg) - 1}, // CheckSum
	}

	// Iterate through requirements if all required
	// tags are present and at correct position
	for _, requirement := range mandatoryTags {
		if _, pos := msg.FindFrom(requirement.t, 0); pos == -1 {
			observations = append(observations, fmt.Sprintf("Missing required Tag [%v]", requirement.t))
			return observations, false
		} else if requirement.p != -1 && pos != requirement.p {
			observations = append(observations, fmt.Sprintf("Expected Tag [%v] at pos %v, found at %v", requirement.t, requirement.p, pos))
			return observations, false
		}
	}

	// Validate BeginString [8]
	beginStr, _ := msg.Get(8)
	if want := router.SessionSpec().BeginString(); beginStr != want {
		observations = append(observations, fmt.Sprintf("BeginString mismatch, expected %v, found %v", want, beginStr))
		return observations, false
	}

	// Mandatory checksum validation
	checksum, _ := msg.Get(10)
	if want := fmt.Sprintf("%03d", msg.Checksum()); want != checksum {
		observations = append(observations, fmt.Sprintf("Checksum validation failed: want %v, got %v",
			want, checksum))
	}

	// Mandatory bodylength validation
	bodylength := msg.BodyLength()
	bodyLenTag, _ := msg.FindFrom(9, 0)
	if got, err := bodyLenTag.AsUint(); err != nil || bodylength != got {
		observations = append(observations, fmt.Sprintf("Bodylength validation failed: want %v, got %v",
			bodylength, got))
	}

	// Route the message correctly to session layer or appl layer
	msgType, _ := msg.Get(35)
	msgSpec := router.SpecForMsgType(msgType)
	msgEntry, ok := msgSpec.Messages[msgType]
	if !ok {
		observations = append(observations, fmt.Sprintf("Unknown MsgType '35=%v'", msgType))
		return observations, false
	}

	// Validate the header (Strict Boundary)
	pos, err := walkSpec(router, msg, mode, router.SessionSpec().Header, nil, 0, &observations)
	if err != nil {
		return observations, false
	}

	// Validate message body (Soft Boundary - only break on trailer tags)
	trailer := router.SessionSpec().Trailer
	pos, err = walkSpec(router, msg, mode, msgEntry, trailer.Lookup, pos, &observations)
	if err != nil {
		return observations, false
	}

	// Validate the trailer (Strict Boundary)
	pos, err = walkSpec(router, msg, mode, trailer, nil, pos, &observations)
	if err != nil {
		return observations, false
	}

	// Any left over fields or if we ran out of "context" of entry supplied
	if pos != len(*msg) {
		observations = append(observations, fmt.Sprintf("Message entry #%v didn't match the spec", pos))
	}

	return observations, len(observations) == 0
}
