package script

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func handleSet(ctx *ScriptContext, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("syntax error, expected `set <key> <value>`")
	}

	key, value := strings.TrimSpace(args[1]), strings.TrimSpace(args[2])
	if _, _, err := ctx.Store.Set(key, value); err != nil {
		return fmt.Errorf("failed to set: %w", err)
	}

	return nil
}

func handlePrint(ctx *ScriptContext, args []string) error {
	fmt.Fprintln(ctx.Writer, strings.Join(args[1:], " "))
	return nil
}

func handleSleep(ctx *ScriptContext, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("syntax error, expected: `sleep <millis>`")
	}

	millis, err := strconv.Atoi(strings.TrimSpace(args[1]))
	if err != nil {
		return fmt.Errorf("not a valid integer: %w", err)
	}

	timer := time.NewTimer(time.Millisecond * time.Duration(millis))
	defer timer.Stop()

	// Wait until timer runs out or context is cancelled
	select {
	case <-ctx.GoCtx.Done():
		return nil
	case <-timer.C:
		return nil
	}
}

func init() {
	RegisterCommand("set", handleSet)     // set <key> <value>
	RegisterCommand("print", handlePrint) // print [<value1>, [<value2> [...]]]
	RegisterCommand("sleep", handleSleep) // sleep <millis>
}
