package mfix

import (
	"strings"
)

// Checksum of the message ignoring tag 10 if present
func Checksum(msg *Message) uint8 {
	return ChecksumIgnoringFields(msg, map[uint16]interface{}{10: nil})
}

// Body length of the mesage, ignoring tags 8, 9 and 10
func BodyLength(msg *Message) uint64 {
	return BodyLengthIgnoringFields(msg, map[uint16]interface{}{8: nil, 9: nil, 10: nil})
}

// Checksum of message, can provide custom tags that needs to be ignored
func ChecksumIgnoringFields(msg *Message, ignoreTags map[uint16]interface{}) uint8 {
	var result int
	for _, field := range *msg {
		if _, ok := ignoreTags[field.Tag]; !ok {
			for _, ch := range field.string() + "\x01" {
				result = result + int(ch)
			}
		}
	}
	return uint8(result % 256)
}

// Body length of the mesage, can provide custom tags that needs to be ignored
func BodyLengthIgnoringFields(msg *Message, ignoreTags map[uint16]interface{}) uint64 {
	var result uint64
	for _, field := range *msg {
		if _, ok := ignoreTags[field.Tag]; !ok {
			result += uint64(len(field.string())) + 1
		}
	}
	return result
}

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

// Ensure input string is as per dtype from input string
func validateDtype(field Field, dtype string) error {
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
