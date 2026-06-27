package main

import (
	cli "github.com/infinage/microfix/cmd/mxshell/internal/cli"
	"os"
	"strings"
)

// Populated from ldflags via GitHub CI/CD
var (
	Version   = "v0.0.0-dev"
	GitCommit = "local"
)

// --- Main Application ---
func main() {
	if args := os.Args; len(args) == 1 {
		// REPL mode
		cli.Repl(Version, GitCommit)
	} else if len(args) >= 3 && args[1] == "-f" && (len(args) == 3 || args[3] == "-v") {
		// Headless script mode
		cli.Script(strings.TrimSpace(args[2]), len(args) == 4)
	} else {
		// Print general help
		cli.PrintHelp(args)
	}
}
