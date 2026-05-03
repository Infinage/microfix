package handlers

import (
	"fmt"
	"strings"

	"github.com/infinage/microfix/internal/mxshell/config"
)

func listAliases(aliases config.Alias) {
	fmt.Println("\n─── Aliases ─────────────────────────────────────")

	if len(aliases) == 0 {
		fmt.Println("  (no aliases defined)")
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	// alignment
	maxLen := 0
	for k := range aliases {
		if len(k) > maxLen {
			maxLen = len(k)
		}
	}

	for k, v := range aliases {
		fmt.Printf("  %-*s : %s\n", maxLen, k, v)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func addAlias(aliases config.Alias, key, value string) {
	fmt.Println("\n─── Alias Update ────────────────────────────────")

	old, exists := aliases[key]
	aliases[key] = value

	fmt.Printf("  Status : OK\n")
	fmt.Printf("  Alias  : %s\n", key)

	if exists {
		fmt.Printf("  Old    : %s\n", old)
	}
	fmt.Printf("  New    : %s\n", value)

	fmt.Println("──────────────────────────────────────────────────")
}

func dumpAliases(aliases config.Alias, filepath string) {
	fmt.Println("\n─── Alias Save ──────────────────────────────────")

	if err := aliases.Dump(filepath); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	} else {
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  File   : %s\n", filepath)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func deleteAlias(aliases config.Alias, keys []string) {
	fmt.Println("\n─── Alias Delete ────────────────────────────────")

	if len(keys) == 0 {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : No alias provided\n")
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	var deleted int

	for _, key := range keys {
		if old, exists := aliases[key]; exists {
			delete(aliases, key)
			fmt.Printf("  Removed: %-15s → %s\n", key, old)
			deleted++
		} else {
			fmt.Printf("  Skipped: %-15s (not found)\n", key)
		}
	}

	if deleted == 0 {
		fmt.Printf("\n  Status : FAILED\n")
	} else {
		fmt.Printf("\n  Status : OK\n")
		fmt.Printf("  Total  : %d removed\n", deleted)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func handleAlias(ctx *AppContext, args []string) {
	aliases := *ctx.Alias

	// default → list
	if len(args) == 1 {
		listAliases(aliases)
		return
	}

	sub := strings.ToLower(args[1])

	switch sub {

	case "list":
		listAliases(aliases)

	case "add":
		if len(args) < 4 {
			fmt.Println("Usage: alias add <name> <fixMessage>")
			return
		}

		key := args[2]
		value := args[3]
		addAlias(aliases, key, value)

	case "save":
		path := ".mxalias"
		if len(args) > 2 {
			path = args[2]
		}
		dumpAliases(aliases, path)

	case "del":
		if len(args) < 2 {
			fmt.Println("Usage: alias del <name1> [<name2> ...]")
			return
		}
		deleteAlias(aliases, args[2:])

	default:
		fmt.Printf("Unknown alias subcommand: %s\n", sub)
	}
}

func init() {
	RegisterCommand(
		"alias",
		handleAlias,
		"View, update or save FIX message shortcuts",
		"alias [list | save <path> | add <alias> <fixMessage> | del <alias>]",
	)
}
