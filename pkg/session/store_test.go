package session

import (
	"slices"
	"testing"

	"github.com/infinage/microfix/pkg/message"
)

func isMsgEqual(msg1 message.Message, msg2 message.Message) bool {
	if len(msg1) != len(msg2) {
		return false
	}
	return slices.EqualFunc(msg1, msg2, func(f1, f2 message.Field) bool {
		return f1.Tag == f2.Tag && f1.Value == f2.Value
	})
}

func TestMessageStore_EmptyStore(t *testing.T) {
	testCases := []struct {
		start int64
		end   int64
	}{{0, 0}, {-1, 0}, {0, 1}, {1, 0}, {1, -1}, {0, -1}, {-1, 1}}

	store := NewMessageStore()
	for _, test := range testCases {
		if res := store.Fetch(test.start, test.end); len(res) != 0 {
			t.Errorf("Expected [].Fetch(%d, %d) to return nil, got %v",
				test.start, test.end, res)
		}
	}
}

func TestMessageStore_EmptyResult(t *testing.T) {
	store := NewMessageStore()
	msg := message.Message{{Tag: 34, Value: "1"}, {Tag: 1, Value: "1"}}
	store.Append(msg)

	testCases := []struct {
		start int64
		end   int64
	}{{2, -1}, {0, -1}, {2, 2}, {2, 3}}

	for _, test := range testCases {
		if res := store.Fetch(test.start, test.end); len(res) != 0 {
			t.Errorf("Expected store.Fetch(%d, %d) to return empty slice, got %v",
				test.start, test.end, res)
		}
	}
}

func TestMessageStore_Append(t *testing.T) {
	store := NewMessageStore()

	msg1 := message.Message{{Tag: 34, Value: "1"}, {Tag: 1, Value: "1"}}
	msg2 := message.Message{{Tag: 34, Value: "2"}, {Tag: 2, Value: "2"}}

	if err := store.Append(msg1); err != nil {
		t.Fatalf("Failed to append msg1: %v", err)
	}
	if err := store.Append(msg2); err != nil {
		t.Fatalf("Failed to append msg2: %v", err)
	}

	if len(store.data) != 2 {
		t.Fatalf("Expected store size 2, got %d", len(store.data))
	}

	if !isMsgEqual(store.data[1], msg1) || !isMsgEqual(store.data[2], msg2) {
		t.Error("Messages stored incorrectly")
	}

	// Test duplicate/backward sequence rejection
	if err := store.Append(msg1); err == nil {
		t.Error("Expected error when appending a message with an older/duplicate SeqNo")
	}
}

func TestMessageStore_FetchSingleMessage(t *testing.T) {
	store := NewMessageStore()
	msg := message.Message{{Tag: 34, Value: "1"}, {Tag: 1, Value: "1"}}
	store.Append(msg)

	if res := store.Fetch(1, 1); len(res) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(res))
	} else if !isMsgEqual(res[0].Msg, msg) {
		t.Error("Expected fetched message to match original")
	}
}

func TestMessageStore_FetchRangeWithGaps(t *testing.T) {
	// Creating a gap at SeqNo 2 (simulating a dropped Admin message)
	msg1 := message.Message{{Tag: 34, Value: "1"}, {Tag: 1, Value: "1"}}
	msg3 := message.Message{{Tag: 34, Value: "3"}, {Tag: 3, Value: "3"}}

	store := NewMessageStore()
	store.Append(msg1)
	store.Append(msg3)

	res := store.Fetch(1, 3)
	if len(res) != 2 {
		t.Fatalf("Expected 2 messages (skipping the gap), got %d", len(res))
	} else if !isMsgEqual(res[0].Msg, msg1) || !isMsgEqual(res[1].Msg, msg3) {
		t.Error("Unexpected fetch result when handling gaps")
	}
}

func TestMessageStore_FetchTillEnd(t *testing.T) {
	msg1 := message.Message{{Tag: 34, Value: "1"}, {Tag: 1, Value: "1"}}
	msg2 := message.Message{{Tag: 34, Value: "2"}, {Tag: 2, Value: "2"}}
	msg3 := message.Message{{Tag: 34, Value: "3"}, {Tag: 3, Value: "3"}}

	store := NewMessageStore()
	store.Append(msg1)
	store.Append(msg2)
	store.Append(msg3)

	// Fetching with endSeqNo 0 means "give me everything up to lastSeqNo"
	if res := store.Fetch(2, 0); len(res) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(res))
	} else if !isMsgEqual(res[0].Msg, msg2) || !isMsgEqual(res[1].Msg, msg3) {
		t.Fatalf("unexpected fetch result")
	}
}
