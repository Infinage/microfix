package shell

import (
	"fmt"
	"strings"

	"github.com/infinage/microfix/pkg/store"
)

func listAliases(st *store.Store) {
	fmt.Println("\n─── Aliases ─────────────────────────────────────")

	// Retreive read only copy of aliases
	aliases := st.Config().Alias

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

func addAlias(key, value string, st *store.Store) {
	fmt.Println("\n─── Alias Update ────────────────────────────────")

	// Call store API
	oldVal, exists, _ := st.Set("ALIAS."+key, value)

	fmt.Printf("  Status : OK\n")
	fmt.Printf("  Alias  : %s\n", key)
	if exists {
		fmt.Printf("  Old    : %s\n", oldVal)
	}
	fmt.Printf("  New    : %s\n", value)

	fmt.Println("──────────────────────────────────────────────────")
}

func deleteAlias(keys []string, st *store.Store) {
	fmt.Println("\n─── Alias Delete ────────────────────────────────")

	if len(keys) == 0 {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : No alias provided\n")
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	var deleted int

	for _, key := range keys {
		if old, exists, _ := st.Unset("ALIAS." + key); exists {
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

func handleAlias(ctx *ShellContext, args []string) {
	// default → list
	if len(args) == 1 {
		listAliases(ctx.Store)
		return
	}

	sub := strings.ToLower(args[1])

	switch sub {

	case "list":
		listAliases(ctx.Store)

	case "add":
		if len(args) < 4 {
			fmt.Println("Usage: alias add <name> <fixMessage>")
			return
		}

		key := args[2]
		value := args[3]
		addAlias(key, value, ctx.Store)

	case "del":
		if len(args) < 2 {
			fmt.Println("Usage: alias del <name1> [<name2> ...]")
			return
		}
		deleteAlias(args[2:], ctx.Store)

	default:
		fmt.Printf("Unknown alias subcommand: %s\n", sub)
	}
}

func init() {
	RegisterCommand(
		"alias",
		handleAlias,
		"View or update FIX message shortcuts",
		"alias [list | add <alias> <fixMessage> | del <alias>]",
	)
}
