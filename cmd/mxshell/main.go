package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/peterh/liner"

	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/store"

	shell "github.com/infinage/microfix/cmd/mxshell/internal/handlers"
)

func historyPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("Failed to resolve UserHomeDir and CurrentWorkingDirectory")
		}
	}
	return path.Join(homeDir, ".mxhistory"), nil
}

func loadHistory(line *liner.State) {
	historyFp, err := historyPath()
	if err != nil {
		return
	}
	if f, err := os.Open(historyFp); err == nil {
		line.ReadHistory(f)
		f.Close()
	}
}

func writeHistory(line *liner.State) {
	historyFp, err := historyPath()
	if err != nil {
		return
	}
	if f, err := os.Create(historyFp); err == nil {
		line.WriteHistory(f)
		f.Close()
	}
}

// Main command completion
func setupAutocomplete(line *liner.State) {
	line.SetCompleter(func(input string) (c []string) {
		// Split the input at the FIRST space only - Case insensitive
		parts := strings.SplitN(strings.ToLower(input), " ", 2)

		// --- Completing the main command (no spaces typed yet) ---
		if len(parts) == 1 {
			cmdPrefix := parts[0]
			for name := range shell.ShellCommandRegistry {
				if strings.HasPrefix(name, cmdPrefix) {
					// If the user typed the exact command, append a space
					// so the cursor jumps forward to accept subcommands
					if name == cmdPrefix {
						c = append(c, name+" ")
					} else {
						c = append(c, name)
					}
				}
			}
			return c
		}

		// --- Completing a subcommand (a space was typed) ---
		cmdName := parts[0]
		argPrefix := parts[1]

		// If there is another space in argPrefix, the user is
		// typing a 3rd arg so we skip autocompletion
		if strings.Contains(argPrefix, " ") {
			return nil
		}

		// Suggest subcommand hints
		if defn, ok := shell.ShellCommandRegistry[cmdName]; ok {
			for _, subCmd := range defn.SubCommands {
				if strings.HasPrefix(subCmd, argPrefix) {
					c = append(c, cmdName+" "+subCmd+" ")
				}
			}
		}

		return c
	})
}

// CLI / REPL mode loop
func replLoop(ctx *shell.ShellContext) {
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

// --- Main Application ---
func main() {
	// Create the config store and shell context
	st := store.InitStore()
	ctx := &shell.ShellContext{
		Store: &st,
		Logs:  ringbuf.NewCircularBuffer(1000),
	}

	if args := os.Args; len(args) == 1 {
		replLoop(ctx)
	} else if len(args) == 3 && args[1] == "-f" {
		shell.RunFile(ctx, strings.TrimSpace(args[2]), os.Stdout)
	} else {
		fmt.Print(
			"MXShell — FIX CLI Client\n\n" +
				"Usage:\n" +
				" mxshell                 Start interactive shell\n" +
				" mxshell -f <file>       Execute script in headless mode\n\n")
	}
}
