package transport

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Assumes input is of the format: 8=FIX*|9=*|.......10=XYZ
func frame(reader *bufio.Reader, sep byte) (string, error) {
	// Read begin string (note: ReadSlice includes delimiter)
	raw, err := reader.ReadSlice(sep)
	if err != nil {
		return "", err
	}
	beginStr := string(raw)
	if len(beginStr) <= 5 || !strings.HasPrefix(beginStr, "8=FIX") {
		return "", fmt.Errorf("Invalid fix begin string, got %v", beginStr)
	}

	// Read bodylen tag
	raw, err = reader.ReadSlice(sep)
	if err != nil {
		return "", err
	}
	bodyLenStr := string(raw)
	if len(bodyLenStr) <= 3 || !strings.HasPrefix(bodyLenStr, "9=") {
		return "", fmt.Errorf("Expected BodyLength tag, got %v", bodyLenStr)
	}

	// Extract body length
	bodyLen, err := strconv.Atoi(bodyLenStr[2 : len(bodyLenStr)-1])
	if err != nil {
		return "", err
	}

	// Read specified no of bytes + "10=XYZ|"
	var bodyRaw = make([]byte, bodyLen+7)
	_, err = io.ReadFull(reader, bodyRaw)
	if err != nil {
		return "", err
	} else if checksum := string(bodyRaw[bodyLen:]); !strings.HasPrefix(checksum, "10=") {
		return "", fmt.Errorf("Expected the fix message to end with checksum [10], got %v", checksum)
	} else if checksum[len(checksum)-1] != sep {
		return "", fmt.Errorf("Fix string must end with %v", sep)
	}

	// Construct the full message and return it
	return beginStr + bodyLenStr + string(bodyRaw), nil
}
