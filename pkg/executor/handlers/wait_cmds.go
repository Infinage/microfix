package script

import (
	"fmt"

	"github.com/infinage/microfix/pkg/ast"
)

func handleExpect(ctx *ScriptContext, args []string) error {
	matcher, err := ast.NewMatcher(args[1:])
	if err != nil {
		return fmt.Errorf("failed to build AST tree: %w", err)
	}

	select {
	case <-ctx.GoCtx.Done():
		return fmt.Errorf("interrupt")
	case <-ctx.Session.Done():
		return fmt.Errorf("session closed")
	case msg, ok := <-ctx.Session.Incoming():
		if !ok {
			return fmt.Errorf("session closed")
		}
		if !matcher.Match(&msg) {
			return fmt.Errorf("expect failed, recevied msg: '%v'", msg.String("|"))
		}
		return nil
	}
}

func handleWait(ctx *ScriptContext, args []string) error {
	matcher, err := ast.NewMatcher(args[1:])
	if err != nil {
		return fmt.Errorf("failed to build AST tree: %w", err)
	}

	for {
		select {
		case <-ctx.GoCtx.Done():
			return fmt.Errorf("interrupt")
		case <-ctx.Session.Done():
			return fmt.Errorf("session closed")
		case msg, ok := <-ctx.Session.Incoming():
			if !ok {
				return fmt.Errorf("session closed")
			}
			if matcher.Match(&msg) {
				return nil
			}
		}
	}
}

func init() {
	RegisterCommand("expect", handleExpect) // expect <MsgLike>
	RegisterCommand("wait", handleWait)     // wait <MsgLike>
}
