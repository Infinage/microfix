package macros

import (
	"regexp"
	"testing"
	"time"
)

func TestSubstituteDate(t *testing.T) {
	today := time.Now()
	todayStr := today.Format("20060102")
	tomorrowStr := today.AddDate(0, 0, 1).Format("20060102")
	yesterdayStr := today.AddDate(0, 0, -1).Format("20060102")

	tests := []struct {
		name      string
		raw       string
		expected  string
		expectErr bool
	}{
		{"Today", "$DATE", todayStr, false},
		{"Tomorrow", "$DATE[1]", tomorrowStr, false},
		{"Explicit Positive", "$DATE[+1]", tomorrowStr, false},
		{"Yesterday", "$DATE[-1]", yesterdayStr, false},
		{"Invalid Format", "$DATE[abc]", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := substituteDate(tt.raw)
			if (err != nil) != tt.expectErr {
				t.Errorf("substituteDate(%q) error = %v, expectErr %v", tt.raw, err, tt.expectErr)
				return
			}
			if res != tt.expected {
				t.Errorf("substituteDate(%q) = %q, want %q", tt.raw, res, tt.expected)
			}
		})
	}
}

func TestSubstituteRandom(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		expectedLen int
		expectErr   bool
		isUUID      bool
	}{
		{"Default UUID", "$UNIQUE", 36, false, true},
		{"Valid Custom Length", "$UNIQUE[15]", 15, false, false},
		{"Valid Max Length Capped", "$UNIQUE[2000]", 1000, false, false},
		{"Invalid Zero Length", "$UNIQUE[0]", 0, true, false},
		{"Invalid Negative Length", "$UNIQUE[-10]", 0, true, false},
		{"Invalid Format", "$UNIQUE[abc]", 0, true, false},
		{"Missing Length Parameter", "$UNIQUE[]", 0, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := substituteUnique(tt.raw)
			if tt.expectErr && err != nil {
				return
			}

			if (err != nil) != tt.expectErr {
				t.Errorf("substituteRandom(%q) error = %v, expectErr %v", tt.raw, err, tt.expectErr)
				return
			}

			if len(res) != tt.expectedLen {
				t.Errorf("Expected length %d, got %d", tt.expectedLen, len(res))
			} else if tt.isUUID {
				matched, err := regexp.MatchString(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`, res)
				if err != nil || !matched {
					t.Errorf("Expected valid UUID format, got: %s", res)
				}
			}
		})
	}

	t.Run("Randomness Check", func(t *testing.T) {
		res1, _ := substituteUnique("$UNIQUE[20]")
		res2, _ := substituteUnique("$UNIQUE[20]")
		if res1 == "" || res2 == "" {
			t.Fatal("substituteRandom returned an empty string")
		} else if res1 == res2 {
			t.Fatalf("Expected consecutive calls to generate unique strings, but got duplicates: %s", res1)
		}
	})
}
