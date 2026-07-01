package cli

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	shell "github.com/infinage/microfix/cmd/mxshell/internal/handlers"
	"github.com/infinage/microfix/pkg/pretty"
	"github.com/peterh/liner"
)

// ReplLoop starts the interactive CLI mode
func Repl(Version, GitCommit string) {
	line := liner.NewLiner()
	defer line.Close()
	defer writeHistory(line)

	fmt.Println(`
 __       __  __    __         ______   __                  __  __ 
|  \     /  \|  \  |  \       /      \ |  \                |  \|  \
| $$\   /  $$| $$  | $$      |  $$$$$$\| $$____   _______  | $$| $$
| $$$\ /  $$$ \$$\/  $$______| $$___\$$| $$    \ /       \ | $$| $$
| $$$$\  $$$$  >$$  $$|      \\$$    \ | $$$$$$$\|  $$$$$$\| $$| $$
| $$\$$ $$ $$ /  $$$$\ \$$$$$$_\$$$$$$\| $$  | $$| $$    $$| $$| $$
| $$ \$$$| $$|  $$ \$$\      |  \__| $$| $$  | $$| $$$$$$$$| $$| $$
| $$  \$ | $$| $$  | $$       \$$    $$| $$  | $$ \$$     \| $$| $$
 \$$      \$$ \$$   \$$        \$$$$$$  \$$   \$$  \$$$$$$$ \$$ \$$
	`)

	// Setup basic autocompletion
	setupAutocomplete(line)

	// Create a new session from shell context
	ctx, err := shell.NewShellContext(Version, GitCommit)
	if err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}

	// Close session and logs broker
	defer ctx.Cleanup()

	// Abort prompts on interupt
	loadHistory(line)

	for {
		input, err := line.Prompt("MFix> ")
		if err != nil {
			break
		} else if input == "" {
			continue
		}

		input = strings.TrimSpace(input)
		creader := csv.NewReader(strings.NewReader(input))
		creader.Comma = ' '
		args, err := creader.Read()
		if err != nil {
			fmt.Printf("Not a valid input: %v\n", err.Error())
			continue
		}

		// Add to history
		line.AppendHistory(input)

		cmdName := strings.ToLower(args[0])
		switch cmdName {
		// Exit on command
		case "exit", "quit":
			return

		// Dispatch to handler
		default:
			if handler, ok := shell.ShellCommandRegistry[cmdName]; ok {
				handler.Handler(ctx, args)
			} else {
				fmt.Printf("Unknown command: %s\n", cmdName)
			}
		}

		fmt.Println()
	}
}

// ScriptMode executes a headless script file
func Script(filename string, verbose bool) {
	// Pass in dummy values for version and git commit
	ctx, err := shell.NewShellContext("", "")
	if err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}

	// Subscribe to logs broker
	if verbose {
		logCh, unsub := ctx.SubscribeLogs()
		defer unsub()

		go func() {
			var sb strings.Builder
			for log := range logCh {
				sb.Reset()
				pretty.Log(&sb, log, ctx.Session().Router())
				fmt.Fprintf(os.Stdout, "\033[90m[LOG] %s\033[0m\n", strings.TrimSpace(sb.String()))
			}
		}()
	}

	err = shell.RunFile(ctx, filename, os.Stdout)
	ctx.Cleanup() // Cleanup on both success / failure

	// Exit code based on execution result
	if err != nil {
		fmt.Printf("run failed: %v\n", err)
		os.Exit(1)
	}
}
