package script

import (
	"fmt"
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

func init() {
	RegisterCommand("expect", handleExpect) // expect <MsgLike>
	RegisterCommand("wait", handleWait)     // wait <MsgLike>
}
