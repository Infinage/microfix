package shell

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/infinage/microfix/pkg/store"
)

func printConfig(st *store.Store) {
	fmt.Println("\n─── Configuration ────────────────────────────────")

	t := reflect.TypeFor[store.Config]()
	v := reflect.ValueOf(st.Config())

	// Find max field name length for alignment
	maxLen := 0
	for i := 0; i < v.NumField(); i++ {
		name := t.Field(i).Name
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if field.Name == "Alias" {
			continue
		}

		value := v.Field(i).Interface()
		fmt.Printf("  %-*s : %v\n", maxLen, field.Name, value)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func loadConfig(fpath string, st *store.Store) {
	fmt.Println("\n─── Config Load ──────────────────────────────────")

	if err := st.LoadConfig(fpath); err == nil {
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  File   : %s\n", fpath)
	} else {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func dumpConfig(fpath string, st *store.Store) {
	fmt.Println("\n─── Config Dump ──────────────────────────────────")

	if err := st.DumpConfig(fpath); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	} else {
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  File   : %s\n", fpath)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func setConfig(key string, value string, st *store.Store) {
	fmt.Println("\n─── Config Update ────────────────────────────────")

	oldVal, _, err := st.Set("CFG."+key, value)
	if err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	fmt.Printf("  Status : OK\n")
	fmt.Printf("  Field  : %s\n", key)
	fmt.Printf("  Old    : %s\n", oldVal)
	fmt.Printf("  New    : %v\n", value)

	fmt.Println("──────────────────────────────────────────────────")
}

// config - list all configs
// config [load|dump] <filepath>
// config set <field> <value>
func handleConfig(ctx *ShellContext, args []string) {
	if len(args) == 1 {
		printConfig(ctx.Store)
		return
	}

	sub := strings.ToLower(args[1])
	switch sub {
	case "load":
		if len(args) < 3 {
			fmt.Println("Usage: config load <path>")
			return
		}
		loadConfig(args[2], ctx.Store)

	case "dump":
		if len(args) < 3 {
			fmt.Println("Usage: config dump <path>")
			return
		}
		dumpConfig(args[2], ctx.Store)

	case "set":
		if len(args) != 4 {
			fmt.Println("Usage: config set <key> <value>")
			return
		}
		setConfig(args[2], args[3], ctx.Store)

	default:
		fmt.Printf("Unknown config subcommand: %s\n", sub)
	}
}

func init() {
	RegisterCommand(
		"config",
		handleConfig,
		"View or modify the current session configuration, config updates are auto saved.",
		"config [load <path> | dump <path> | set <key> <val>]",
	)
}
