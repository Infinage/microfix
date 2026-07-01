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
	sess := ctx.Session()
	logCh, unsubscribe, err := sess.SubscribeLog()
	if err != nil {
		return Falsy(fmt.Errorf("failed to create wire tap: %w", err))
	}
	defer unsubscribe()

	// Timeout after configured duration - if set to 0 assumes no timeout
	var timeout <-chan time.Time
	if timeoutSec := ctx.Store.Config().DefaultTimeoutSec; timeoutSec > 0 {
		timeout = time.After(time.Duration(timeoutSec) * time.Second)
	}

	for {
		select {
		case <-ctx.GoCtx.Done():
			return fmt.Errorf("interrupt")
		case <-sess.Done():
			return Falsy(fmt.Errorf("session not active"))
		case <-timeout:
			return Falsy(fmt.Errorf("timeout"))
		case log, ok := <-logCh:
			if !ok {
				return Falsy(fmt.Errorf("session closed"))
			} else if log.Type == session.LogRecv {
				if matcher.Match(&log.Msg) {
					return nil
				}

				// Lenient in our checks for Heartbeat & TestRequest
				if mt, _ := log.Msg.Get(35); mt == "0" || mt == "1" {
					continue
				}

				err := fmt.Errorf("assertion failed [expect], received msg: '%v'", log.Msg.String("|"))
				return Falsy(err)
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
	sess := ctx.Session()
	logCh, unsubscribe, err := sess.SubscribeLog()
	if err != nil {
		return Falsy(fmt.Errorf("failed to create wire tap: %w", err))
	}
	defer unsubscribe()

	// Timeout after configured duration - if set to 0 assumes no timeout
	var timeout <-chan time.Time
	if timeoutSec := ctx.Store.Config().DefaultTimeoutSec; timeoutSec > 0 {
		timeout = time.After(time.Duration(timeoutSec) * time.Second)
	}

	for {
		select {
		case <-ctx.GoCtx.Done():
			return fmt.Errorf("interrupt")
		case <-sess.Done():
			return Falsy(fmt.Errorf("session not active"))
		case <-timeout:
			return Falsy(fmt.Errorf("timeout"))
		case log, ok := <-logCh:
			if !ok {
				return Falsy(fmt.Errorf("session closed"))
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

	// Helper to check from snapshot as a fallback
	sess := ctx.Session()
	targetState := strings.ToLower(args[1])
	checkFromSnap := func() bool {
		snap := sess.Status()
		currentState := strings.ToLower(snap.State.String())
		return currentState == targetState
	}

	// Subscribe to logs and on failure check from tombstone
	logCh, unsubscribe, err := sess.SubscribeLog()
	if err != nil {
		if checkFromSnap() {
			return nil
		}
		return fmt.Errorf("failed to create wire tap: %w", err)
	}
	defer unsubscribe()

	// safegaurd against scenario when currentState transitions
	// to targetstate before log subscription succeeds
	if checkFromSnap() {
		return nil
	}

	// Timeout after configured duration - if set to 0 assumes no timeout
	var timeout <-chan time.Time
	if timeoutSec := ctx.Store.Config().DefaultTimeoutSec; timeoutSec > 0 {
		timeout = time.After(time.Duration(timeoutSec) * time.Second)
	}

	for {
		select {
		case <-ctx.GoCtx.Done():
			return fmt.Errorf("interrupt")
		case <-sess.Done():
			if checkFromSnap() { // Check from tombstone
				return nil
			}
			return Falsy(fmt.Errorf("session closed while waiting for status: %s", targetState))
		case <-timeout:
			return Falsy(fmt.Errorf("timeout"))
		case log, ok := <-logCh:
			if !ok {
				return Falsy(fmt.Errorf("session closed"))
			} else if log.Type == session.LogTran {
				if targetState == strings.ToLower(log.States[1]) {
					return nil
				}
			}
		}
	}
}

func init() {
	RegisterCommand("expect", handleExpect)         // expect <MsgLike>
	RegisterCommand("wait", handleWait)             // wait <MsgLike>
	RegisterCommand("waitstatus", handleWaitStatus) // waitstatus <state>
}
