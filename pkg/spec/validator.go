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
// It returns the index of the next unprocessed field, or an error if validation fails.
func walkSpec(ro *Router, msg *message.Message, mode ValidationMode, context Entry,
	fields map[uint16]FieldDef, idx int, obs *[]string) (int, error) {

	// localLookup tracks expected tags in the current context.
	// As we process tags, we remove them from this map to track missing required tags.
	localLookup := maps.Clone(context.Lookup)

	// Store any out of context tags and if we have pending mandatory tags in context
	// it means that we have encountered an out of context tag that we need to report
	oocTagIdx := -1

	for idx < len(*msg) {
		field := (*msg)[idx]
		pos, exists := localLookup[field.Tag]

		// --- Context Boundary & Unknown Tag Handling ---
		if !exists {
			// Tag is completely unknown to the global dictionary.
			if _, knownField := ro.Field(field.Tag); !knownField {
				if mode == ValidationStrict {
					*obs = append(*obs, fmt.Sprintf("Unknown tag [%v]", field.Tag))
				}
				idx++
				continue
			}

			// Tag is known globally, but doesn't belong in this specific group/message.
			// This signals that the current context (e.g., repeating group) has ended.
			// We break out and let the parent context handle this tag.
			oocTagIdx = idx
			break
		}

		// --- Process Valid Field ---
		entry := context.Entries[pos]
		delete(localLookup, field.Tag) // Marking as visited

		// Validate data type
		if mode == ValidationStrict {
			if err := validateDtype(field, fields[field.Tag].Type); err != nil {
				*obs = append(*obs, fmt.Sprintf("Datatype validation failed for tag [%v]", field.Tag))
			}
		}

		// --- Handle Repeating Groups ---
		if entry.IsGroup {
			repeat, err := field.AsUint()
			if err != nil {
				err = fmt.Errorf("Expected group tag to have integer value, got %v", field.Value)
				*obs = append(*obs, err.Error())
				return idx, err
			}

			// Preserve the group order across repeating groups
			var group1Start, groupSize = idx + 1, -1

			for gi := range repeat {
				// Recurse for that repeating group
				idx, err = walkSpec(ro, msg, mode, entry, fields, idx+1, obs)
				if err != nil {
					return idx, err
				}

				// For the first grp repetition, establish the blueprint (size and anchor tag)
				if groupSize == -1 {
					// Store the begin and end indices of a group
					groupSize = idx - group1Start

					// Ensure first tag in group is our anchor tag from spec
					anchorTag := (*msg)[group1Start].Tag
					if anchorPos, found := entry.Lookup[anchorTag]; !found || anchorPos != 0 {
						*obs = append(*obs, fmt.Sprintf("Tag %v immediately following groupno missing"+
							" or not at first position on Group Spec", (*msg)[idx+1].Tag))
					}
				} else if mode == ValidationStrict {
					// Validate the ordering for second repeating group onwards
					groupStart := group1Start + (int(gi) * groupSize)
					for i := range groupSize {
						g0, g := (*msg)[group1Start+i], (*msg)[groupStart+i]
						if g0 != g {
							*obs = append(*obs, fmt.Sprintf("Expected group #%v entry #%v to be %v, had %v",
								gi+1, i+1, g0.Tag, g.Tag))
						}
					}
				}

			}

			// Walk spec already updated idx to point just after current scope
			continue
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
				oocTagIdx = -1
			}
			*obs = append(*obs, fmt.Sprintf("Missing required field tag [%v]", tag))
		}
	}

	// Fail the check if any observations in current context
	if len(*obs) > 0 {
		return idx, fmt.Errorf("Observed %v issues processing message", len(*obs))
	}

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

	// Validate the header
	pos, err := walkSpec(router, msg, mode, router.SessionSpec().Header, router.SessionSpec().Fields, 0, &observations)
	if err != nil {
		return observations, false
	}

	// Validate the message body following header, we start off where header finished
	pos, err = walkSpec(router, msg, mode, msgEntry, msgSpec.Fields, pos, &observations)
	if err != nil {
		return observations, false
	}

	// Validate the trailer, start off where body validation left us
	pos, err = walkSpec(router, msg, mode, router.SessionSpec().Trailer, router.SessionSpec().Fields, pos, &observations)
	if err != nil {
		return observations, false
	}

	// Any left over fields or if we ran out of "context" of entry supplied
	if pos != len(*msg) {
		observations = append(observations, fmt.Sprintf("Message entry #%v didn't match the spec", pos))
	}

	return observations, len(observations) == 0
}
