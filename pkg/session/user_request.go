package session

import (
	"time"

	"github.com/infinage/microfix/pkg/message"
)

type userRequest interface {
	apply(sess *Session)
}

type snapshotRequest struct {
	reply chan Snapshot
}

func (r snapshotRequest) apply(sess *Session) {
	r.reply <- sess.engine.Snapshot()
}

type closeRequest struct{}

func (r closeRequest) apply(sess *Session) {
	sess.writeLog(newInfoLog(time.Now(), "Close initiated by user/engine"))
	sess.engine.off()
	sess.base.Close()
}

type messageSendRequest struct {
	passthrough bool
	message     message.Message
}

func (r messageSendRequest) apply(sess *Session) {
	sess.handleSend(r.message, r.passthrough)
}

type resetSequenceRequest struct {
	inSeqNum  int64
	outSeqNum int64
}

func (r resetSequenceRequest) apply(sess *Session) {
	actions := sess.engine.OnResetSequence(r.inSeqNum, r.outSeqNum)
	sess.execute(actions)
}

type lastMessageRequest struct {
	isIncoming bool
	msgType    string
	reply      chan *message.Message
}

func (r lastMessageRequest) apply(sess *Session) {
	// If not found return nil
	var reply *message.Message
	if r.isIncoming {
		if msg, ok := sess.lastIn[r.msgType]; ok {
			reply = &msg
		}
	} else {
		if msg, ok := sess.lastOut[r.msgType]; ok {
			reply = &msg
		}
	}
	r.reply <- reply
}
