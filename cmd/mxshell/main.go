package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/infinage/microfix/internal/mxshell/config"
	"github.com/infinage/microfix/internal/mxshell/handlers"
	"github.com/infinage/microfix/pkg/ringbuf"

	"github.com/peterh/liner"
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

// --- Main Application ---

func main() {
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

	cfg := config.InitConfig()
	alias := config.InitAlias()

	ctx := &handlers.AppContext{
		Alias:  &alias,
		Config: &cfg,
		Logs:   ringbuf.NewCircularBuffer(1000),
	}

	sess, err := handlers.NewSession(&cfg)
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

		// REPL stays alive
		case "disconnect":
			ctx.Session.Close()

		// Clear screen
		case "clear":
			fmt.Print("\033[H\033[2J")

		// Dispatch to handler
		default:
			if handler, ok := handlers.CommandRegistry[cmdName]; ok {
				handler.Handler(ctx, args)
			} else {
				fmt.Printf("Unknown command: %s\n", cmdName)
			}
		}

		fmt.Println()
	}
}
