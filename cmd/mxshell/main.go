package main

import (
	"os"
	"strings"

	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/store"

	cli "github.com/infinage/microfix/cmd/mxshell/internal/cli"
	shell "github.com/infinage/microfix/cmd/mxshell/internal/handlers"
)

// --- Main Application ---
func main() {
	// Create the config store and shell context
	st := store.InitStore()
	ctx := &shell.ShellContext{
		Store: &st,
		Logs:  ringbuf.NewCircularBuffer(1000),
	}

	if args := os.Args; len(args) == 1 {
		// REPL mode
		cli.Repl(ctx)
	} else if len(args) >= 3 && args[1] == "-f" && (len(args) == 3 || args[3] == "-v") {
		// Headless script mode
		cli.Script(ctx, strings.TrimSpace(args[2]), len(args) == 4)
	} else {
		// Print general help
		cli.PrintHelp(args)
	}
}
