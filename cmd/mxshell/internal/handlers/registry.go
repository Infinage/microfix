package shell

import (
	"strings"
	"sync"

	"github.com/infinage/microfix/pkg/broker"
	"github.com/infinage/microfix/pkg/pretty"
	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/store"
)

// Every handler takes in the context along with args
type ShellContext struct {
	Version   string
	GitCommit string
	Store     *store.Store
	Logs      *ringbuf.CircularBuffer

	session     *session.Session
	mu          sync.RWMutex
	logBroker   *broker.Broker
	closeLogger func()
}

// Read from broker and write into circular buffer
func startLogger(lbroker *broker.Broker, cb *ringbuf.CircularBuffer, routerFn func() *spec.Router) func() {
	// Subscribe to the log broker and listen on it
	logCh, unsubscribe := lbroker.Subscribe()
	go func() {
		var sb strings.Builder
		for log := range logCh {
			sb.Reset()
			pretty.Log(&sb, log, routerFn())
			cb.Write(strings.TrimSpace(sb.String()))
		}
	}()

	return unsubscribe
}

func NewShellContext(Version, GitCommit string) (*ShellContext, error) {
	// Load store from file or create one if missing
	st := store.InitStore()

	sess, err := NewSession(&st)
	if err != nil {
		return nil, err
	}

	lbroker := broker.NewBroker()
	if err := lbroker.Bind(sess); err != nil {
		return nil, err
	}

	cb := ringbuf.NewCircularBuffer(1000)
	ctx := &ShellContext{
		Version:   Version,
		GitCommit: GitCommit,
		Store:     &st,
		Logs:      cb,
		session:   sess,
		logBroker: lbroker,
	}

	// Start a goroutine listening on brokers channel
	// Broker is persisted across resets and outlives session
	routerFn := func() *spec.Router { return ctx.Session().Router() }
	ctx.closeLogger = startLogger(lbroker, cb, routerFn)

	return ctx, nil
}

func (ctx *ShellContext) SubscribeLogs() (<-chan session.Log, func()) {
	return ctx.logBroker.Subscribe()
}

func (ctx *ShellContext) Cleanup() {
	ctx.Session().Close()
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	if ctx.closeLogger != nil {
		ctx.closeLogger()
	}
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

	// Attempt to bind log broker to new session
	err = ctx.logBroker.Bind(newSess)
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
