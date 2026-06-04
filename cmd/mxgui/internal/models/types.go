package models

import (
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/store"
)

type DashboardData struct {
	Snap   session.Snapshot
	Config store.Config
}

type ModalData struct {
	IpAddr string
	Port   uint16
}

type SendMessageData struct {
	Router  *spec.Router
	Aliases *map[string]string
}
