package shell

import (
	"fmt"
	"io"
	"os"
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
	scriptCtx, stop := executor.NewScriptContext(ctx.Session, ctx.Store, out)
	defer stop()

	// Evaluate the file
	if err = executor.EvalBatch(f, &scriptCtx); err != nil {
		fmt.Printf("run command failed: %v\n", err)
	}
}

func init() {
	RegisterCommand("disconnect", handleDisconnect, "Disconnect session", "disconnect")
	RegisterCommand("clear", handleClear, "Clear screen", "clear")
	RegisterCommand("run", handleScript, "Run an external script", "run [-q] <filepath>")
}
