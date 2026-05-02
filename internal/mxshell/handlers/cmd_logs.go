package handlers

import (
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/infinage/microfix/pkg/ringbuf"
)

// Show logs to screen until user interupts
func handleLogStream(cb *ringbuf.CircularBuffer) {
	ch, cancel := cb.Subscribe()
	defer cancel()

	fmt.Println("\n--- Log Stream (Ctrl+C to exit) ---")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)

	for {
		select {
		case <-sigChan:
			fmt.Println("\n--- Exiting Stream ---")
			return

		case logLine, ok := <-ch:
			if !ok {
				return
			}
			fmt.Println(logLine)
		}
	}
}

// Filter out the logs based on regex and print to screen
func handleLogSearch(cb *ringbuf.CircularBuffer, pattern string) {
	filtered, err := cb.Filter(pattern)
	if err != nil {
		fmt.Println(err.Error())
	}

	for _, line := range filtered {
		fmt.Println(line)
	}

	fmt.Printf("\n--- Log Search: '%s' ---\n", pattern)
	if len(filtered) == 0 {
		fmt.Println("No matches in buffer.")
	} else {
		fmt.Printf("\nTotal matches: %d\n", len(filtered))
	}
}

// Main log handler
func handleLogs(ctx *AppContext, args []string) {
	if len(args) < 2 {
		fmt.Println("\n--- Session Logs ---")
		ctx.Logs.Dump(os.Stdout)
		return
	}

	sub := strings.ToLower(args[1])
	switch sub {
	case "-f":
		handleLogStream(ctx.Logs)
	case "search":
		if len(args) < 3 {
			fmt.Println("Usage: logs search <regex>")
		} else {
			handleLogSearch(ctx.Logs, args[2])
		}
	default:
		fmt.Printf("Unknown logs subcommand: %s\n", sub)
	}
}

func init() {
	RegisterCommandHandler("logs", handleLogs)
}
