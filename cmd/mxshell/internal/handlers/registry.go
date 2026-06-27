package shell

import (
	"sync"

	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// Every handler takes in the context along with args
type ShellContext struct {
	Version   string
	GitCommit string
	Store     *store.Store
	Logs      *ringbuf.CircularBuffer

	session *session.Session
	mu      sync.RWMutex
}

func NewShellContext(Version, GitCommit string) (*ShellContext, error) {
	// Load store from file or create one if missing
	st := store.InitStore()

	sess, err := NewSession(&st)
	if err != nil {
		return nil, err
	}

	return &ShellContext{
		Version:   Version,
		GitCommit: GitCommit,
		Store:     &st,
		Logs:      ringbuf.NewCircularBuffer(1000),
		session:   sess,
	}, nil
}

func (ctx *ShellContext) Session() *session.Session {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.session
}

// Safely reset session
func (ctx *ShellContext) resetSession() error {
	// Create a new session from latest config
	newSess, err := NewSession(ctx.Store)
	if err != nil {
		return err
	}

	// Reset session
	ctx.mu.Lock()
	oldSess := ctx.session
	ctx.session = newSess
	ctx.mu.Unlock()

	// Close the old session
	if oldSess != nil {
		oldSess.Close()
	}

	return nil
}

// Typing a command handler
type Command func(ctx *ShellContext, args []string)

// Type enforce what can be registered
type CommandDef struct {
	Handler     Command
	Description string
	Usage       string
	SubCommands []string
}

// Map containing commands and corresponding handlers
var ShellCommandRegistry = make(map[string]CommandDef)

// Global func to register new handlers
func RegisterCommand(command string, handler Command, desc, usage string, subCmds []string) {
	ShellCommandRegistry[command] = CommandDef{
		Handler:     handler,
		Description: desc,
		Usage:       usage,
		SubCommands: subCmds,
	}
}
