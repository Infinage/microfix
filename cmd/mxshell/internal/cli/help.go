package cli

import (
	"fmt"
	"github.com/infinage/microfix/pkg/executor"
)

func PrintHelp(args []string) {
	// Print general help
	fmt.Print(
		"MXShell — FIX CLI Client\n\n" +
			"Usage:\n" +
			"  mxshell                  Start interactive shell\n" +
			"  mxshell -f <file> [-v]   Execute script in headless mode (-v for verbose logs)\n" +
			"  mxshell -h               Display help\n\n")

	// Detailed help with script syntax document
	if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
		fmt.Print("----------------------------------------------------------\n\n" +
			"MXShell Scripting Reference\n" + executor.ScriptHelpText + "\n")
	}
}
