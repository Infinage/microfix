package handlers

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/infinage/microfix/pkg/ringbuf"
)

// Show logs to screen until user interupts
func streamLogs(ctx *ShellContext) {
	ch, unsubscribe := ctx.Session.SubscribeLog()
	defer unsubscribe()

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

func tailLogs(cb *ringbuf.CircularBuffer, n int) {
	if n <= 0 {
		fmt.Println("tail count must be > 0")
		return
	}

	fmt.Println("\n─── Log Tail ───────────────────────────────────")

	logs := cb.Tail(n)
	for _, line := range logs {
		fmt.Println(line)
	}

	fmt.Println("──────────────────────────────────────────────────")
	fmt.Printf("  Count : %d\n", len(logs))
	fmt.Println("──────────────────────────────────────────────────")
}

func headLogs(cb *ringbuf.CircularBuffer, n int) {
	if n <= 0 {
		fmt.Println("head count must be > 0")
		return
	}

	fmt.Println("\n─── Log Head ───────────────────────────────────")

	logs := cb.Head(n)
	for _, line := range logs {
		fmt.Println(line)
	}

	fmt.Println("──────────────────────────────────────────────────")
	fmt.Printf("  Count : %d\n", len(logs))
	fmt.Println("──────────────────────────────────────────────────")
}

func clearLogs(cb *ringbuf.CircularBuffer) {
	fmt.Println("\n─── Clear Logs -─────────────────────────────────")
	cb.Clear()
	fmt.Printf("  Status : OK\n")
	fmt.Println("──────────────────────────────────────────────────")
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

func saveLogs(cb *ringbuf.CircularBuffer, filepath string) {
	fmt.Println("\n─── Save Logs ─────────────────────────────────────")

	f, err := os.Create(filepath)
	if err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("──────────────────────────────────────────────────")
		return
	}
	defer f.Close()

	for _, line := range cb.Dump() {
		fmt.Fprintln(f, line)
	}

	fmt.Printf("  Status : OK\n")
	fmt.Printf("  Path   : %s\n", filepath)
	fmt.Println("──────────────────────────────────────────────────")
}

// Main log handler
func handleLogs(ctx *ShellContext, args []string) {
	if len(args) < 2 {
		fmt.Println("\n─── Session Logs ─────────────────────────────────")
		for _, line := range ctx.Logs.Dump() {
			fmt.Fprintln(os.Stdout, line)
		}
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	sub := strings.ToLower(args[1])
	switch sub {
	case "stream":
		streamLogs(ctx)
	case "clear":
		clearLogs(ctx.Logs)
	case "head", "tail":
		if len(args) != 3 {
			fmt.Println("Usage: logs [head | tail] <n>")
		} else if n, err := strconv.Atoi(args[2]); err != nil {
			fmt.Printf("Not a valid integer: %v\n", args[2])
		} else if args[1] == "head" {
			headLogs(ctx.Logs, n)
		} else {
			tailLogs(ctx.Logs, n)
		}
	case "search":
		if len(args) < 3 {
			fmt.Println("Usage: logs search <regex>")
		} else {
			searchLogs(ctx.Logs, args[2])
		}
	case "save":
		if len(args) < 3 {
			fmt.Println("Usage: logs save <filepath>")
		} else {
			saveLogs(ctx.Logs, args[2])
		}
	default:
		fmt.Printf("Unknown logs subcommand: %s\n", sub)
	}
}

func init() {
	RegisterCommand(
		"logs",
		handleLogs,
		"View, stream, search, or save session logs",
		"logs [stream | search <regex> | save <path> | clear | head <n> | tail <n>]",
	)
}
