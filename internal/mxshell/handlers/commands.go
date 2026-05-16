package handlers

import (
	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// Every handler takes in the context along with args
type AppContext struct {
	Session *session.Session
	Store   *store.Store
	Logs    *ringbuf.CircularBuffer
}

// Type enforce what can be registered
type Command struct {
	Handler     func(ctx *AppContext, args []string)
	Description string
	Usage       string
}

// Map containing commands and corresponding handlers
var CommandRegistry = make(map[string]Command)

// Global func to register new handlers
func RegisterCommand(command string, handler func(*AppContext, []string), desc, usage string) {
	CommandRegistry[command] = Command{
		Handler:     handler,
		Description: desc,
		Usage:       usage,
	}
}
