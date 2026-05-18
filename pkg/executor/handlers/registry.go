package script

import (
	"context"
	"io"

	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// Every handler takes in the context
type ScriptContext struct {
	GoCtx   context.Context  // Context to cancel running scripts
	Session *session.Session // Executor runs commands on the Session object
	Store   *store.Store     // Configs and Runtime variables
	Writer  io.Writer        // Writing output
}

type Command func(ctx *ScriptContext, args []string) error

// Map containing commands and corresponding handlers
var ScriptRegistry = map[string]Command{}

// Global func to register new handlers
func RegisterCommand(name string, handler Command) {
	ScriptRegistry[name] = handler
}
