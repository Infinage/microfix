package handlers

import (
	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// Every handler takes in the context along with args
type ShellContext struct {
	Session *session.Session
	Store   *store.Store
	Logs    *ringbuf.CircularBuffer
}

// Typing a command handler
type Command func(ctx *ShellContext, args []string)

// Type enforce what can be registered
type CommandDef struct {
	Handler     Command
	Description string
	Usage       string
}

// Map containing commands and corresponding handlers
var ShellCommandRegistry = make(map[string]CommandDef)

// Global func to register new handlers
func RegisterCommand(command string, handler Command, desc, usage string) {
	ShellCommandRegistry[command] = CommandDef{
		Handler:     handler,
		Description: desc,
		Usage:       usage,
	}
}
