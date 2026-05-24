package cli

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	shell "github.com/infinage/microfix/cmd/mxshell/internal/handlers"
	"github.com/infinage/microfix/pkg/pretty"
	"github.com/infinage/microfix/pkg/session"
	"github.com/peterh/liner"
)

// ReplLoop starts the interactive CLI mode
func Repl(ctx *shell.ShellContext) {
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
	sess, err := shell.NewSession(ctx.Store)
	if err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}

	// Set session into the context
	ctx.Session = sess

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
			ctx.Session.Close()
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
func Script(ctx *shell.ShellContext, filename string, verbose bool) {
	// Initialize a new session with config loaded (default if missing)
	cfg := ctx.Store.Config()
	var err error
	ctx.Session, err = session.NewSession(
		cfg.SessionSpec,
		cfg.SenderCompID,
		cfg.TargetCompID,
		cfg.HeartbeatInt,
		session.EngineOptions{
			DefaultApplVer:   cfg.ApplicationSpec,
			SkipLatencyCheck: cfg.SkipLatencyCheckInValidate,
		})

	if err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}

	// Subscribe to logs
	if verbose {
		logCh, unsub, err := ctx.Session.SubscribeLog()
		if err != nil {
			fmt.Printf("Failed to subscribe to logs: %v\n", err)
			os.Exit(1)
		}
		defer unsub()

		go func() {
			var sb strings.Builder
			for log := range logCh {
				sb.Reset()
				pretty.Log(&sb, log, ctx.Session.Router())
				fmt.Fprintf(os.Stdout, "\033[90m[LOG] %s\033[0m\n", strings.TrimSpace(sb.String()))
			}
		}()
	}

	if err := shell.RunFile(ctx, filename, os.Stdout); err != nil {
		fmt.Printf("run failed: %v\n", err)
		os.Exit(1)
	}
}
