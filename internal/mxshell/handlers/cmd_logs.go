package handlers

import (
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/infinage/microfix/pkg/ringbuf"
)

// Show logs to screen until user interupts
func streamLogs(cb *ringbuf.CircularBuffer) {
	ch, cancel := cb.Subscribe()
	defer cancel()

	fmt.Println("\n─── Log Stream (Ctrl+C to exit) ────────────────")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)

	for {
		select {
		case <-sigChan:
			fmt.Println("\n─── Exiting Stream ─────────────────────────────")
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
func searchLogs(cb *ringbuf.CircularBuffer, pattern string) {
	filtered, err := cb.Filter(pattern)
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Println("\n─── Log Search ─────────────────────────────────")
	fmt.Printf("  Pattern : %s\n\n", pattern)

	for _, line := range filtered {
		fmt.Println(line)
	}

	fmt.Println("\n──────────────────────────────────────────────────")
	fmt.Printf("  Matches : %d\n", len(filtered))
	fmt.Println("──────────────────────────────────────────────────")
}

// Main log handler
func handleLogs(ctx *AppContext, args []string) {
	if len(args) < 2 {
		fmt.Println("\n─── Session Logs ─────────────────────────────────")
		ctx.Logs.Dump(os.Stdout)
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	sub := strings.ToLower(args[1])
	switch sub {
	case "-f":
		streamLogs(ctx.Logs)
	case "search":
		if len(args) < 3 {
			fmt.Println("Usage: logs search <regex>")
		} else {
			searchLogs(ctx.Logs, args[2])
		}
	default:
		fmt.Printf("Unknown logs subcommand: %s\n", sub)
	}
}

func init() {
	RegisterCommandHandler("logs", handleLogs)
}
