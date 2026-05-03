package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/infinage/microfix/internal/mxshell/config"
	"github.com/infinage/microfix/internal/mxshell/handlers"
	"github.com/infinage/microfix/pkg/ringbuf"
)

// --- Main Application ---

func main() {
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

	ctx.Session = sess
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("MFix> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		creader := csv.NewReader(strings.NewReader(input))
		creader.Comma = ' '
		args, err := creader.Read()
		if err != nil {
			fmt.Printf("Not a valid input: %v\n", err.Error())
			continue
		}

		cmdName := strings.ToLower(args[0])
		switch cmdName {
		// Exit on command
		case "exit", "quit":
			ctx.Session.Close()
			return

		// REPL stays alive
		case "disconnect":
			ctx.Session.Close()

		// Claer screen
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
