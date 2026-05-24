package cli

import (
	"strings"

	shell "github.com/infinage/microfix/cmd/mxshell/internal/handlers"
	"github.com/infinage/microfix/pkg/store"
	"github.com/peterh/liner"
)

// Main command completion
func setupAutocomplete(line *liner.State) {
	line.SetCompleter(func(input string) (c []string) {
		// Split the input at the FIRST space only - Case insensitive
		parts := strings.SplitN(strings.ToLower(input), " ", 3)

		// --- Completing the main command (no spaces typed yet) ---
		if len(parts) == 1 {
			cmdPrefix := parts[0]
			for name := range shell.ShellCommandRegistry {
				if strings.HasPrefix(name, cmdPrefix) {
					c = append(c, name+" ")
				}
			}
			return c
		}

		// --- Suggesting subcommand hints ---
		if len(parts) == 2 {
			cmdName, subCmdPrefix := parts[0], parts[1]
			if defn, ok := shell.ShellCommandRegistry[cmdName]; ok {
				for _, subCmd := range defn.SubCommands {
					if strings.HasPrefix(subCmd, subCmdPrefix) {
						c = append(c, cmdName+" "+subCmd+" ")
					}
				}
			}
		}

		// --- Autocomplete for "config set" ---
		if len(parts) == 3 {
			cmdName, subCmdName, configPrefix := parts[0], parts[1], parts[2]
			if cmdName == "config" && subCmdName == "set" {
				for _, cfgName := range store.ConfigFields() {
					if strings.HasPrefix(strings.ToLower(cfgName), configPrefix) {
						c = append(c, cmdName+" "+subCmdName+" "+cfgName+" ")
					}
				}

			}
		}

		return c
	})
}
