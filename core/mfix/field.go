package mfix

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Base struct for entire fix
type Field struct {
	Tag   uint16
	Value string
}

// Helper to write to string (internal)
func (f *Field) string() string {
	return strconv.Itoa(int(f.Tag)) + "=" + f.Value
}

// Support FIX's custom monthyear struct
type MonthYear struct {
	Year  int
	Month time.Month
	Day   int // Optional
	Week  int // Optional for YYYYMMwN
}

// Convert to int64, explictly disallow numbers starting with '+'
func (f *Field) AsInt() (int64, error) {
	switch {
	case len(f.Value) == 0:
		return 0, errors.New("Empty string")
	case f.Value[0] != '-' && !(f.Value[0] >= '0' && f.Value[0] <= '9'):
		return 0, errors.New("Must start with a digit or -ve sign")
	default:
		return strconv.ParseInt(f.Value, 10, 64)
	}
}

// Convert to uint64, explictly disallow numbers starting with '+'
func (f *Field) AsUint() (uint64, error) {
	switch {
	case len(f.Value) == 0:
		return 0, errors.New("Empty string")
	case !(f.Value[0] >= '0' && f.Value[0] <= '9'):
		return 0, errors.New("Must start with a digit")
	default:
		return strconv.ParseUint(f.Value, 10, 64)
	}
}

// Convert to float64
func (f *Field) AsDouble() (float64, error) {
	switch {
	case len(f.Value) == 0:
		return 0, errors.New("Empty string")
	case f.Value[0] != '-' && !(f.Value[0] >= '0' && f.Value[0] <= '9'):
		return 0, errors.New("Must start with a digit or -ve sign")
	default:
		return strconv.ParseFloat(f.Value, 64)
	}
}

// Single character only
func (f *Field) AsChar() (rune, error) {
	if utf8.RuneCountInString(f.Value) != 1 {
		return 0, errors.New("Field value contains multiple chars")
	}
	return rune(f.Value[0]), nil
}

// Input can be 'Y' or 'N'
func (f *Field) AsBool() (bool, error) {
	switch f.Value {
	case "Y":
		return true, nil
	case "N":
		return false, nil
	default:
		return false, errors.New("Not a valid boolean, expected Y/N")
	}
}

// Space seperated unique list of characters
func (f *Field) AsCharVector() ([]rune, error) {
	length := utf8.RuneCountInString(f.Value)
	if length == 0 || length%2 == 0 {
		return nil, errors.New("Empty string or contains even no of chars")
	}

	var uniq = make(map[rune]any)
	var res = make([]rune, 0, length)
	for i, ch := range f.Value {
		switch {
		case i%2 == 1 && ch != ' ':
			return res, fmt.Errorf("Expected whitespace (%v) @ %v, got '%v' (%v)", ' ', i, string(ch), ch)
		case i%2 == 0:
			if _, ok := uniq[ch]; ok {
				return res, fmt.Errorf("Duplicate char found %v", string(ch))
			}
			uniq[ch] = nil
			res = append(res, ch)
		}
	}

	return res, nil
}

// Space seperated list of unique strings
func (f *Field) AsStringVector() ([]string, error) {
	if len(f.Value) == 0 {
		return nil, errors.New("Empty string or contains even no of chars")
	}

	var uniq = make(map[string]any)
	res := strings.Split(f.Value, " ")
	for _, tok := range res {
		if tok == "" {
			return nil, errors.New("invalid spacing")
		}
		if _, ok := uniq[tok]; ok {
			return res, fmt.Errorf("Duplicate string found %v", tok)
		}
		uniq[tok] = nil
	}

	return res, nil
}

// Format: yyyyMMdd
func (f *Field) AsDate() (time.Time, error) {
	return time.Parse("20060102", f.Value)
}

// Format: HH:MM:SS or HH:MM:SS.mmm
func (f *Field) AsTime() (time.Time, error) {
	return time.Parse("15:04:05", f.Value)
}

// Format: HH:MM[:ss][Z|[+|–hh[:mm]]]
func (f *Field) AsTZTime() (time.Time, error) {
	val := f.Value
	if val == "" {
		return time.Time{}, errors.New("empty time string")
	}

	// Explicitly check for milliseconds
	if strings.Contains(val, ".") {
		return time.Time{}, errors.New("milliseconds not supported in TZTime")
	}

	var layout string
	hasZ := strings.HasSuffix(val, "Z")
	hasOffset := strings.ContainsAny(val, "+-")
	if !hasZ && !hasOffset {
		return time.Time{}, errors.New("TZTime requires Z or offset")
	}

	// Determine base layout (HH:MM:SS vs HH:MM)
	// count colons to see if we have seconds
	colons := strings.Count(val, ":")
	base := "15:04"
	if colons >= 2 {
		base = "15:04:05"
	}

	// Append the correct Zone directive
	if hasZ {
		layout = base + "Z"
	} else if offset := val[strings.LastIndexAny(val, "+-"):]; strings.Count(offset, ":") > 0 {
		layout = base + "Z07:00"
	} else {
		layout = base + "Z07"
	}

	t, err := time.Parse(layout, val)
	if err != nil {
		return time.Time{}, err
	}

	// Validation: Extract offset in seconds
	_, offset := t.Zone()
	absOffset := offset
	if absOffset < 0 {
		absOffset = -absOffset
	}

	// 12 hours = 43200 seconds
	if absOffset > 12*3600 {
		return time.Time{}, fmt.Errorf("timezone offset out of range: %d seconds", offset)
	}

	return t, nil
}

// Format: yyyyMMdd-HH:mm:ss[.SSS][Z|[+|–hh[:oo]]]
func (f *Field) AsTZTimestamp() (time.Time, error) {
	val := f.Value
	layout := "20060102-15:04:05"

	// Handle sub-seconds
	if strings.Contains(val, ".") {
		layout += ".000"
	}

	// Handle Zulu
	if strings.HasSuffix(val, "Z") {
		return time.Parse(layout+"Z", val)
	}

	// Handle Offset - Detect if there's a colon in the offset portion
	// We look at the part after the time (which ends at index 17 or more)
	idx := strings.LastIndexAny(val, "+-")
	if idx != -1 && strings.Contains(val[idx:], ":") {
		layout += "Z07:00"
	} else {
		layout += "Z07"
	}

	return time.Parse(layout, val)
}

// Format: YYYYMMDD or YYYYMMWW (W = 1-5)
func (f *Field) AsMonthYear() (MonthYear, error) {
	val, res := f.Value, MonthYear{}

	if len(val) < 6 {
		return res, errors.New("invalid MonthYear length")
	}

	// Parse YYYYMM
	t, err := time.Parse("200601", val[:6])
	if err != nil {
		return res, err
	}

	res.Year, res.Month = t.Year(), t.Month()

	if len(val) == 8 {
		if val[6] == 'w' {
			// Case: YYYYMMwN
			week, err := strconv.Atoi(val[7:])
			if err != nil || week < 1 || week > 5 {
				return res, errors.New("invalid week format")
			}
			res.Week = week
		} else {
			// Case: YYYYMMDD
			day, err := strconv.Atoi(val[6:])
			if err != nil {
				return res, err
			}
			res.Day = day
		}
	}
	return res, nil
}
