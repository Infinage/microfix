package handlers

import (
	"github.com/infinage/microfix/internal/mxshell/config"
	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/session"
)

// Every handler takes in the context along with args
type AppContext struct {
	Session *session.Session
	Config  *config.Config
	Logs    *ringbuf.CircularBuffer
}

// Type enforce what can be registered
type CommandHandler func(ctx *AppContext, args []string)

// Map containing commands and corresponding handlers
var CommandRegistry = make(map[string]CommandHandler)

// Global func to register new handlers
func RegisterCommandHandler(command string, handler CommandHandler) {
	CommandRegistry[command] = handler
}
