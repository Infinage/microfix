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

// walkSpec recursively validates a FIX message context (Header, Body, Trailer, or Group).
// Returns the index of the next unprocessed field or a structural parsing error.
func walkSpec(ro *Router, msg *message.Message, vmode ValidationMode, context Entry,
	terminators map[uint16]int, idx int, obs *[]string) (int, error) {

	// Track expected tags to identify missing required fields later
	localLookup := maps.Clone(context.Lookup)
	oocTagIdx := -1 // Tracks premature termination by unexpected tags

	for idx < len(*msg) {
		field := (*msg)[idx]
		pos, exists := localLookup[field.Tag]

		// --- Context Boundary & Unknown Tag Handling ---
		if !exists {
			// Tag is UNKNOWN to the global dictionary.
			_, knownField := ro.Field(field.Tag)
			if !knownField {
				if vmode == ValidationStrict {
					*obs = append(*obs, fmt.Sprintf("Unknown tag [%v]", field.Tag))
				}
				idx++
				continue
			}

			// Hard Boundary: Terminate on outer terminators, or OOC tags inside Strict Groups
			_, isTerminal := terminators[field.Tag]
			if (context.IsGroup && vmode == ValidationStrict) || isTerminal {
				oocTagIdx = idx
				break
			}

			// Soft Boundary: OOC tag in a normal message block (Header/Body/Trailer)
			if vmode == ValidationStrict {
				*obs = append(*obs, fmt.Sprintf("Unexpected out-of-context tag [%v]", field.Tag))
			}
			idx++
			continue
		}

		// --- Process Valid Field ---
		entry := context.Entries[pos]
		delete(localLookup, field.Tag) // Marking as visited

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
				err = fmt.Errorf("Expected group tag [%v] to have integer value, got '%v'", field.Tag, field.Value)
				*obs = append(*obs, err.Error())
				return idx, err
			}

			idx++ // Advance past the group count tag
			if idx >= len(*msg) {
				*obs = append(*obs, fmt.Sprintf("Message truncated immediately after group count tag [%v]", field.Tag))
				return idx, nil
			}

			group1Start, groupSize := idx, -1

			// Group terminators include the anchor tag and outer context terminators
			anchorTag := (*msg)[group1Start].Tag
			terminatorsForGroup := map[uint16]int{anchorTag: -1}
			maps.Copy(terminatorsForGroup, terminators)
			maps.Copy(terminatorsForGroup, localLookup)

			for gi := range repeat {
				// Recurse for that repeating group.
				groupStart := idx

				idx, err = walkSpec(ro, msg, vmode, entry, terminatorsForGroup, idx, obs)
				if err != nil {
					return idx, err // Bubble up structural failures
				}

				// For the first grp repetition, establish the blueprint (size and anchor tag)
				if groupSize == -1 {
					// Store the begin and end indices of a group
					groupSize = idx - group1Start
					if groupSize == 0 {
						*obs = append(*obs, fmt.Sprintf("Empty group [%d] with non zero counts [%d]",
							field.Tag, repeat))
						break
					}

					// Ensure first tag in group is our anchor tag
					if anchorPos, found := entry.Lookup[anchorTag]; !found || anchorPos != 0 {
						var expectedAnchorTag uint16
						for k, v := range entry.Lookup {
							if v == 0 {
								expectedAnchorTag = k
								break
							}
						}
						*obs = append(*obs, fmt.Sprintf("Repeating group delimiter mismatch: expected tag [%v]"+
							" as first field, got [%v]", expectedAnchorTag, anchorTag))
					}
				} else if vmode == ValidationStrict {
					if cgroupSize := idx - groupStart; cgroupSize != groupSize {
						// Ensure group sizes are identical
						*obs = append(*obs, fmt.Sprintf("Group [%d] Entry #%d has %d fields(s), expected %d "+
							"(based on Entry #1)", field.Tag, gi+1, cgroupSize, groupSize))
					} else {
						// Validate group tag ordering matches the blueprint
						for i := range groupSize {
							g0, g := (*msg)[group1Start+i], (*msg)[groupStart+i]
							if g0.Tag != g.Tag {
								*obs = append(*obs, fmt.Sprintf("Expected group #%v entry #%v to be "+
									"tag [%v], had [%v]", gi+1, i+1, g0.Tag, g.Tag))
							}
						}
					}
				}

			}
			continue // walkSpec already updated idx
		}

		idx++
	}

	// --- Missing Required Tags Check ---
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
		observations = append(observations, fmt.Sprintf("Unknown MsgType '35=%s'", msgType))
		return observations, false
	}

	header := router.SessionSpec().Header
	trailer := router.SessionSpec().Trailer

	// Header ends when Body / Trailer starts (msg could end up having 0 body tags)
	headerTerminators := make(map[uint16]int)
	maps.Copy(headerTerminators, msgEntry.Lookup)
	maps.Copy(headerTerminators, trailer.Lookup)

	// Validate the header
	pos, err := walkSpec(router, msg, mode, header, headerTerminators, 0, &observations)
	if err != nil {
		return observations, false
	}

	// Validate message body
	pos, err = walkSpec(router, msg, mode, msgEntry, trailer.Lookup, pos, &observations)
	if err != nil {
		return observations, false
	}

	// Validate the trailer (there is a potential issue where we could fit in
	// arbitrary tags between trailer start and checksum in basic validation mode)
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
