package diff

import (
	"reflect"
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

// Helper to construct fields
func f(tag uint16, val string) message.Field {
	return message.Field{Tag: tag, Value: val}
}

func TestDiff_Basic(t *testing.T) {
	t.Run("Source & Target empty", func(t *testing.T) {
		msg1, msg2 := message.Message{}, message.Message{}
		diffs := Compare(msg1, msg2)
		if size := len(diffs); size != 0 {
			t.Errorf("Expected empty diff, got: %d", size)
		}
	})

	t.Run("Source empty", func(t *testing.T) {
		msg1, msg2 := message.Message{}, message.Message{f(35, "V")}
		diffs := Compare(msg1, msg2)
		if size := len(diffs); size != 1 {
			t.Errorf("Expected a single diff, got %d", size)
		} else if d := diffs[0]; d.Tag != 35 || d.Source != "" || d.Target != "V" || d.Status != DiffAdded {
			t.Errorf("Expected Tag=35, Source=\"\", Target=\"V\", Status=DiffAdded; got %v", d)
		}
	})

	t.Run("Target empty", func(t *testing.T) {
		msg1, msg2 := message.Message{f(35, "V")}, message.Message{}
		diffs := Compare(msg1, msg2)
		if size := len(diffs); size != 1 {
			t.Errorf("Expected a single diff, got %d", size)
		} else if d := diffs[0]; d.Tag != 35 || d.Source != "V" || d.Target != "" || d.Status != DiffRemoved {
			t.Errorf("Expected Tag=35, Source=\"V\", Target=\"\", Status=DiffRemoved; got %v", d)
		}
	})
}

func TestDiff_Extended(t *testing.T) {
	t.Run("Exact Match", func(t *testing.T) {
		msg := message.Message{f(8, "FIXT.1.1"), f(35, "A"), f(108, "30")}
		diffs := Compare(msg, msg)

		expected := []DiffRow{
			{Tag: 8, Source: "FIXT.1.1", Target: "FIXT.1.1", Status: DiffEqual},
			{Tag: 35, Source: "A", Target: "A", Status: DiffEqual},
			{Tag: 108, Source: "30", Target: "30", Status: DiffEqual},
		}

		if !reflect.DeepEqual(diffs, expected) {
			t.Errorf("Expected exact match, got %+v", diffs)
		}
	})

	t.Run("Modified Values", func(t *testing.T) {
		msg1 := message.Message{f(35, "A"), f(108, "30")}
		msg2 := message.Message{f(35, "A"), f(108, "60")} // 108 modified
		diffs := Compare(msg1, msg2)

		expected := []DiffRow{
			{Tag: 35, Source: "A", Target: "A", Status: DiffEqual},
			{Tag: 108, Source: "30", Target: "60", Status: DiffModified},
		}

		if !reflect.DeepEqual(diffs, expected) {
			t.Errorf("Expected modified 108, got %+v", diffs)
		}
	})

	t.Run("Interspersed Additions and Removals", func(t *testing.T) {
		msg1 := message.Message{f(35, "D"), f(11, "ID1"), f(21, "1")}    // 21 removed in msg2
		msg2 := message.Message{f(35, "D"), f(11, "ID1"), f(60, "TIME")} // 60 added in msg2

		diffs := Compare(msg1, msg2)

		expected := []DiffRow{
			{Tag: 35, Source: "D", Target: "D", Status: DiffEqual},
			{Tag: 11, Source: "ID1", Target: "ID1", Status: DiffEqual},
			{Tag: 21, Source: "1", Target: "", Status: DiffRemoved},
			{Tag: 60, Source: "", Target: "TIME", Status: DiffAdded},
		}

		if !reflect.DeepEqual(diffs, expected) {
			t.Errorf("Expected interspersed add/remove, got %+v", diffs)
		}
	})

	t.Run("Repeating Groups Handling", func(t *testing.T) {
		// msg1 has 2 repeating entries
		msg1 := message.Message{
			f(35, "W"),
			f(268, "2"),
			f(269, "0"), f(270, "150"), // Entry 1
			f(269, "1"), f(270, "151"), // Entry 2
		}

		// msg2 has 3 repeating entries, and Entry 1's price changed
		msg2 := message.Message{
			f(35, "W"),
			f(268, "3"),                // Modified
			f(269, "0"), f(270, "149"), // Entry 1 (Modified Price)
			f(269, "1"), f(270, "151"), // Entry 2 (Equal)
			f(269, "2"), f(270, "152"), // Entry 3 (Added)
		}

		diffs := Compare(msg1, msg2)

		expected := []DiffRow{
			{Tag: 35, Source: "W", Target: "W", Status: DiffEqual},
			{Tag: 268, Source: "2", Target: "3", Status: DiffModified},
			{Tag: 269, Source: "0", Target: "0", Status: DiffEqual},
			{Tag: 270, Source: "150", Target: "149", Status: DiffModified},
			{Tag: 269, Source: "1", Target: "1", Status: DiffEqual},
			{Tag: 270, Source: "151", Target: "151", Status: DiffEqual},
			{Tag: 269, Source: "", Target: "2", Status: DiffAdded},
			{Tag: 270, Source: "", Target: "152", Status: DiffAdded},
		}

		if !reflect.DeepEqual(diffs, expected) {
			t.Errorf("Repeating group diff failed.\nExp: %+v\nGot: %+v", expected, diffs)
		}
	})

	t.Run("Complete Mismatch", func(t *testing.T) {
		msg1 := message.Message{f(35, "A"), f(108, "30")}
		msg2 := message.Message{f(35, "0"), f(112, "REQ")}
		diffs := Compare(msg1, msg2)

		// Since Tags 35 match, LCS will anchor on 35 and treat it as modified,
		// 108 as removed, and 112 as added.
		expected := []DiffRow{
			{Tag: 35, Source: "A", Target: "0", Status: DiffModified},
			{Tag: 108, Source: "30", Target: "", Status: DiffRemoved},
			{Tag: 112, Source: "", Target: "REQ", Status: DiffAdded},
		}

		if !reflect.DeepEqual(diffs, expected) {
			t.Errorf("Expected complete mismatch handling, got %+v", diffs)
		}
	})
}
