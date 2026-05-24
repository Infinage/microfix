package script

import (
	"fmt"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/ast"
	"github.com/infinage/microfix/pkg/session"
)

func handleExpect(ctx *ScriptContext, args []string) error {
	matcher, err := ast.NewMatcher(args[1:])
	if err != nil {
		return fmt.Errorf("failed to build AST tree: %w", err)
	}

	// Subscribe to logs so that we dont steal app msg
	logCh, unsubscribe, err := ctx.Session.SubscribeLog()
	if err != nil {
		return fmt.Errorf("failed to create wire tap: %w", err)
	}
	defer unsubscribe()

	// Timeout after configured duration
	timeout := time.After(time.Duration(ctx.Store.Config().DefaultTimeoutSec) * time.Second)

	for {
		select {
		case <-ctx.GoCtx.Done():
			return fmt.Errorf("interrupt")
		case <-ctx.Session.Done():
			return fmt.Errorf("session closed")
		case <-timeout:
			return fmt.Errorf("timeout")
		case log, ok := <-logCh:
			if !ok {
				return fmt.Errorf("session closed")
			} else if log.Type == session.LogRecv {
				if matcher.Match(&log.Msg) {
					return nil
				}

				// Lenient in our checks for Heartbeat & TestRequest
				if mt, _ := log.Msg.Get(35); mt == "0" || mt == "1" {
					continue
				}

				return fmt.Errorf("assertion failed [expect], received msg: '%v'", log.Msg.String("|"))
			}
		}
	}
}

func handleWait(ctx *ScriptContext, args []string) error {
	matcher, err := ast.NewMatcher(args[1:])
	if err != nil {
		return fmt.Errorf("failed to build AST tree: %w", err)
	}

	// Subscribe to logs so that we dont steal app msg
	logCh, unsubscribe, err := ctx.Session.SubscribeLog()
	if err != nil {
		return fmt.Errorf("failed to create wire tap: %w", err)
	}
	defer unsubscribe()

	// Timeout after configured duration
	timeout := time.After(time.Duration(ctx.Store.Config().DefaultTimeoutSec) * time.Second)

	for {
		select {
		case <-ctx.GoCtx.Done():
			return fmt.Errorf("interrupt")
		case <-ctx.Session.Done():
			return fmt.Errorf("session closed")
		case <-timeout:
			return fmt.Errorf("timeout")
		case log, ok := <-logCh:
			if !ok {
				return fmt.Errorf("session closed")
			} else if log.Type == session.LogRecv {
				if matcher.Match(&log.Msg) {
					return nil
				}
			}
		}
	}
}

func handleWaitStatus(ctx *ScriptContext, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: `waitstatus <StateName>`")
	}

	// Create a ticker to poll the session state
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Use a timeout context so the test doesn't hang forever
	timeout := time.After(5 * time.Second)

	// Case insensitive comparisons
	targetState := strings.ToLower(args[1])

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for status: %s", targetState)
		case <-ticker.C:
			snap := ctx.Session.Status()
			currentState := strings.ToLower(snap.State.String())
			if currentState == targetState {
				return nil
			}
		}
	}
}

func init() {
	RegisterCommand("expect", handleExpect)         // expect <MsgLike>
	RegisterCommand("wait", handleWait)             // wait <MsgLike>
	RegisterCommand("waitstatus", handleWaitStatus) // waitstatus <state>
}
