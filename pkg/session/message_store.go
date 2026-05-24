package session

import (
	"fmt"
	"github.com/infinage/microfix/pkg/message"
)

type MessageStore struct {
	data      map[int64]message.Message
	lastSeqNo int64
}

type storeEntry struct {
	Msg   message.Message
	seqNo int64
}

func NewMessageStore() MessageStore {
	return MessageStore{
		data:      make(map[int64]message.Message),
		lastSeqNo: 0,
	}
}

func (store *MessageStore) Append(msgClone message.Message) error {
	seqNoTag, pos := msgClone.FindFrom(34, 0)
	if pos == -1 {
		return fmt.Errorf("Missing tag 34")
	}

	seqNo, err := seqNoTag.AsInt()
	if err != nil {
		return err
	} else if seqNo <= store.lastSeqNo {
		return fmt.Errorf("Attempt to insert message with SeqNo <= %d", seqNo)
	}

	store.lastSeqNo = seqNo
	store.data[seqNo] = msgClone
	return nil
}

func (store *MessageStore) Reset() {
	store.data = make(map[int64]message.Message)
	store.lastSeqNo = 0
}

func (store *MessageStore) Fetch(begSeqNo, endSeqNo int64) []storeEntry {
	if endSeqNo == 0 || endSeqNo > store.lastSeqNo {
		endSeqNo = store.lastSeqNo
	}

	var result []storeEntry
	for seqNo := begSeqNo; seqNo <= endSeqNo; seqNo++ {
		if msg, ok := store.data[seqNo]; ok {
			result = append(result, storeEntry{Msg: msg, seqNo: seqNo})
		}
	}

	return result
}
