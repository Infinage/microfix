package executor

import (
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

type Context struct {
	sess *session.Session
	st   *store.Store
}
