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
	sess.writeLog(newSysEventLog(time.Now(), "Close initiated by user/engine"))
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

type resetSequence struct {
	inSeqNum  int64
	outSeqNum int64
}

func (r resetSequence) apply(sess *Session) {
	sess.engine.OnResetSequence(r.inSeqNum, r.outSeqNum)
}
