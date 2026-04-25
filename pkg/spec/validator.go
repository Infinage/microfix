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

// Validate an input message and return list of observations
func (spec *Spec) Validate(message *message.Message, mode ValidationMode) (bool, []string) {
	var observations []string
	if mode == ValidationNone {
		return true, observations
	}

	msgType, ok := message.Get(35)
	if !ok {
		observations = append(observations, "MsgType Tag (35) missing")
		return false, observations
	}

	// Checksum validation if required
	if _, ok := spec.Trailer.Lookup[10]; ok {
		if checksum, ok := message.Get(10); !ok {
			observations = append(observations, "Missing checksum tag [10]")
		} else if want := fmt.Sprintf("%03d", message.Checksum()); want != checksum {
			observations = append(observations, fmt.Sprintf("Checksum validation failed: want %v, got %v",
				want, checksum))
		}
	}

	// Bodylength validation if required
	if _, ok := spec.Header.Lookup[9]; ok {
		bodylength := message.BodyLength()
		bodyLenTag, pos := message.FindFrom(9, 0)
		if pos == -1 {
			observations = append(observations, "Missing bodylength tag [9]")
		} else if got, err := bodyLenTag.AsUint(); err != nil || bodylength != got {
			observations = append(observations, fmt.Sprintf("Bodylength validation failed: want %v, got %v",
				bodylength, got))
		}
	}

	msgSpec, ok := spec.Messages[msgType]
	if !ok {
		observations = append(observations, fmt.Sprintf("Unknown MsgType '35=%v'", msgType))
		return false, observations
	}

	// Walk through and validate for entries against header, msg body and trailer
	var err error
	var pos int
	pos, err = walkSpec(message, spec.Header, 0, &observations, spec.Fields, mode)
	if err != nil {
		return false, observations
	}
	pos, err = walkSpec(message, msgSpec, pos, &observations, spec.Fields, mode)
	if err != nil {
		return false, observations
	}
	pos, err = walkSpec(message, spec.Trailer, pos, &observations, spec.Fields, mode)
	if err != nil {
		return false, observations
	}

	if pos != len(*message) {
		observations = append(observations, fmt.Sprintf("Message entry #%v didn't match the spec", pos))
	}

	return len(observations) == 0, observations
}

// Returns index just after processing the message for that context
func walkSpec(msg *message.Message, context Entry, idx int, obs *[]string,
	fields map[uint16]FieldDef, mode ValidationMode) (int, error) {

	// Clone the original so we don't end up modifying it
	localLookup := maps.Clone(context.Lookup)

	for idx < len(*msg) {
		// Get the field and look it up from spec
		field := (*msg)[idx]
		pos, exists := localLookup[field.Tag]

		if !exists {
			// If unknown field we can skip processing it
			if _, knownField := fields[field.Tag]; !knownField {
				if mode == ValidationStrict {
					*obs = append(*obs, fmt.Sprintf("Unknown tag [%v]", field.Tag))
				}
				idx++
				continue
			}

			// If known field, either we are in the wrong context (group has ended)
			// or message is malformed and we have to stop short
			// We would assert that we have processed all entries and would fail
			// validation in this scenario in 'Validate()'
			break
		}

		// Get a copy of the entry from spec and mark as visited
		entry := context.Entries[pos]
		delete(localLookup, field.Tag)

		// Validate data type
		if mode == ValidationStrict {
			if err := validateDtype(field, fields[field.Tag].Type); err != nil {
				*obs = append(*obs, fmt.Sprintf("Datatype validation failed for tag [%v]", field.Tag))
			}
		}

		// If group, recurse into it specified no of times
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
				idx, err = walkSpec(msg, entry, idx+1, obs, fields, mode)
				if err != nil {
					return idx, err
				}

				// For the first repeating group
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

	// Check for required tags still pending processing
	for tag, pos := range localLookup {
		if context.Entries[pos].Required {
			*obs = append(*obs, fmt.Sprintf("Missing required field tag [%v]", tag))
		}
	}

	// Fail the check if any observations in current context
	if len(*obs) > 0 {
		return idx, fmt.Errorf("Observed %v issues processing message", len(*obs))
	}

	return idx, nil
}
