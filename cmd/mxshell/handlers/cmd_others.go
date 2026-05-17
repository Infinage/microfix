package handlers

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/infinage/microfix/pkg/executor"
)

func handleDisconnect(ctx *ShellContext, args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: disconnect")
		return
	}
	ctx.Session.Close()
}

func handleClear(_ *ShellContext, args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: clear")
		return
	}
	fmt.Print("\033[H\033[2J")
}

func handleScript(ctx *ShellContext, args []string) {
	if nargs := len(args); nargs < 2 || nargs > 3 || (nargs == 3 && strings.TrimSpace(args[1]) != "-q") {
		fmt.Println("Usage: run [-q] <filepath>")
		return
	}

	fpath := strings.TrimSpace(args[1])
	var out io.Writer = os.Stdout

	if len(args) == 3 {
		fpath = strings.TrimSpace(args[2])
		out = io.Discard
	}

	f, err := os.Open(fpath)
	if err != nil {
		fmt.Println("failed to read: %w", err)
	}

	// Trigger context close on interupt
	goCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Evaluate the file
	err = executor.EvalBatch(f, &executor.ScriptContext{
		GoCtx:   goCtx,
		Session: ctx.Session,
		Store:   ctx.Store,
		Writer:  out,
	})

	if err != nil {
		fmt.Println("run command failed: %w", err)
	}
}

func init() {
	RegisterCommand("disconnect", handleDisconnect, "Disconnect session", "disconnect")
	RegisterCommand("clear", handleClear, "Clear screen", "clear")
	RegisterCommand("run", handleScript, "Run an external script", "run [-q] <filepath>")
}
