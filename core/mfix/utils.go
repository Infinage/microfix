package mfix

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
		if _, ok := ignoreTags[field.tag]; !ok {
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
		if _, ok := ignoreTags[field.tag]; !ok {
			result += uint64(len(field.string())) + 1
		}
	}
	return result
}
