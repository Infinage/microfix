package mfix

import (
	"testing"
	"time"
)

func TestField_GENERIC_STRING(t *testing.T) {
	f := Field{Value: "Hello"}
	if f.Value != "Hello" {
		t.Errorf("value = %v, want Hello", f.Value)
	}
}

func TestField_AMT(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"23.23", 23.23},
		{"0023.2300", 23.23},
		{"-23.23", -23.23},
		{"25", 25.0},
	}
	for _, tt := range tests {
		f := Field{Value: tt.input}
		val, err := f.AsDouble()
		if err != nil {
			t.Errorf("AsDouble(%q) unexpected error: %v", tt.input, err)
		}
		if val != tt.want {
			t.Errorf("AsDouble(%q) = %v, want %v", tt.input, val, tt.want)
		}
	}

	invalid := []string{" 23", "+23", "10.0a", "", ".", "23..", ".23."}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsDouble(); err == nil {
			t.Errorf("AsDouble(%q) should have failed", val)
		}
	}
}

func TestField_BOOLEAN(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Y", true},
		{"N", false},
	}
	for _, tt := range tests {
		f := Field{Value: tt.input}
		val, err := f.AsBool()
		if err != nil {
			t.Errorf("AsBool(%q) unexpected error: %v", tt.input, err)
		}
		if val != tt.want {
			t.Errorf("AsBool(%q) = %v, want %v", tt.input, val, tt.want)
		}
	}

	invalid := []string{"y", "n", "true", "t", "", "YN", "NY"}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsBool(); err == nil {
			t.Errorf("AsBool(%q) should have failed", val)
		}
	}
}

func TestField_CHAR(t *testing.T) {
	tests := []struct {
		input string
		want  rune
	}{
		{"a", 'a'},
		{"!", '!'},
	}
	for _, tt := range tests {
		f := Field{Value: tt.input}
		val, err := f.AsChar()
		if err != nil {
			t.Errorf("AsChar(%q) unexpected error: %v", tt.input, err)
		}
		if val != tt.want {
			t.Errorf("AsChar(%q) = %c, want %c", tt.input, val, tt.want)
		}
	}

	invalid := []string{"ab", ""}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsChar(); err == nil {
			t.Errorf("AsChar(%q) should have failed", val)
		}
	}
}

func TestField_DATA(t *testing.T) {
	raw := "\x01\x00\x01\x00\x01\n\r"
	f := Field{Value: raw}
	if len(f.Value) != 7 {
		t.Errorf("Length = %v, want 7", len(f.Value))
	}
	if f.Value != raw {
		t.Errorf("Data mismatch")
	}
}

func TestField_INT(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"23", 23},
		{"00023", 23},
		{"-23", -23},
		{"-00023", -23},
	}
	for _, tt := range tests {
		f := Field{Value: tt.input}
		val, err := f.AsInt()
		if err != nil {
			t.Errorf("AsInt(%q) error: %v", tt.input, err)
		}
		if val != tt.want {
			t.Errorf("AsInt(%q) = %v, want %v", tt.input, val, tt.want)
		}
	}

	invalid := []string{"+23", "23.0", "10a", "", " 23", "999999999999999999999"}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsInt(); err == nil {
			t.Errorf("AsInt(%q) should have failed", val)
		}
	}
}

func TestField_MULTIPLECHARVALUE(t *testing.T) {
	f1 := Field{Value: "2 A F"}
	res1, err := f1.AsCharVector()
	if err != nil || len(res1) != 3 || res1[0] != '2' || res1[1] != 'A' || res1[2] != 'F' {
		t.Errorf("AsCharVector('2 A F') failed: %v, error: %v", res1, err)
	}

	f2 := Field{Value: "2"}
	res2, err := f2.AsCharVector()
	if err != nil || len(res2) != 1 || res2[0] != '2' {
		t.Errorf("AsCharVector('2') failed: %v", err)
	}

	invalid := []string{"2 2", "2 AF", "", "A ", " A", " A ", "a  a", "  "}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsCharVector(); err == nil {
			t.Errorf("AsCharVector(%q) should have failed", val)
		}
	}
}

func TestField_MULTIPLESTRINGVALUE(t *testing.T) {
	// Valid case 1
	f1 := Field{Value: "2 A F"}
	r1, err := f1.AsStringVector()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r1) != 3 || r1[0] != "2" || r1[1] != "A" || r1[2] != "F" {
		t.Errorf("AsStringVector('2 A F') failed: %v", r1)
	}

	// Valid case 2
	f2 := Field{Value: "2A"}
	r2, err := f2.AsStringVector()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r2) != 1 || r2[0] != "2A" {
		t.Errorf("AsStringVector('2A') failed: %v", r2)
	}

	// Invalid cases
	invalid := []string{"AA AA", "", "A ", " A", "a  a", "  "}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsStringVector(); err == nil {
			t.Errorf("AsStringVector(%q) should have failed", val)
		}
	}
}

func TestField_LOCALMKTDATE(t *testing.T) {
	f := Field{Value: "20240101"}
	got, err := f.AsDate()
	if err != nil {
		t.Fatalf("AsDate error: %v", err)
	}
	if got.Year() != 2024 || got.Month() != time.January || got.Day() != 1 {
		t.Errorf("AsDate = %v, want 2024-01-01", got)
	}

	invalid := []string{"20241301", "20241234", "20241234a", "20241210 ", "", "202401", "2024-01-01"}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsDate(); err == nil {
			t.Errorf("AsDate(%q) should have failed", val)
		}
	}
}

func TestField_TZTIMEONLY(t *testing.T) {
	tests := []struct {
		input  string
		wantH  int
		wantM  int
		wantS  int
		wantOf int // seconds
	}{
		{"01:02Z", 1, 2, 0, 0},
		{"01:02:03Z", 1, 2, 3, 0},
		{"01:02:03+01", 1, 2, 3, 3600},
		{"01:02:03+01:00", 1, 2, 3, 3600},
		{"01:02:03-01:00", 1, 2, 3, -3600},
		{"01:02:03+01:30", 1, 2, 3, 5400},
	}

	for _, tt := range tests {
		f := Field{Value: tt.input}
		got, err := f.AsTZTime()
		if err != nil {
			t.Errorf("AsTZTime(%q) error: %v", tt.input, err)
			continue
		}
		_, off := got.Zone()
		if got.Hour() != tt.wantH || got.Minute() != tt.wantM || got.Second() != tt.wantS || off != tt.wantOf {
			t.Errorf("AsTZTime(%q) = %v:%v:%v Off:%v", tt.input, got.Hour(), got.Minute(), got.Second(), off)
		}
	}

	invalid := []string{"01:02:03", "01:02:03.444Z", "", "01:02Z+01", "01:02:03+13", "01:02:03-13",
		"1:2:3+0160", "01:02:03+12:01", "01:02:03+", "01:02:03+0", "01:02:03+000", "01:02:03+01:00abc",
		"2006090113:09:00Z", "20060901-24:00:00Z", "20060901-23:60:00Z"}

	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsTZTime(); err == nil {
			t.Errorf("AsTZTime(%q) should have failed", val)
		}
	}
}

func TestField_TZTIMESTAMP(t *testing.T) {
	tests := []struct {
		input  string
		wantY  int
		wantH  int
		wantOf int
		wantMs int
	}{
		{"20060901-07:39:00Z", 2006, 7, 0, 0},
		{"20060901-02:39:00-05", 2006, 2, -18000, 0},
		{"20060901-13:09:00+05:30", 2006, 13, 19800, 0},
		{"20060901-13:09:00.123+05:30", 2006, 13, 19800, 123},
	}

	for _, tt := range tests {
		f := Field{Value: tt.input}
		got, err := f.AsTZTimestamp()
		if err != nil {
			t.Errorf("AsTZTimestamp(%q) error: %v", tt.input, err)
			continue
		}
		_, off := got.Zone()
		ms := got.Nanosecond() / 1e6
		if got.Year() != tt.wantY || got.Hour() != tt.wantH || off != tt.wantOf || ms != tt.wantMs {
			t.Errorf("AsTZTimestamp(%q) failed: Year %v, Hour %v, Off %v, Ms %v", tt.input, got.Year(), got.Hour(), off, ms)
		}
	}

	invalid := []string{"20060901-07:39:00", "", "20060901", "20060901T07:39:00Z"}
	for _, val := range invalid {
		f := Field{Value: val}
		if _, err := f.AsTZTimestamp(); err == nil {
			t.Errorf("AsTZTimestamp(%q) should have failed", val)
		}
	}
}
