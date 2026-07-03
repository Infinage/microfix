package shell

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/infinage/microfix/pkg/store"
)

func printConfig(st *store.Store) {
	fmt.Println("\n─── Configuration ───────────────────────────────────────────────")

	t := reflect.TypeFor[store.Config]()
	v := reflect.ValueOf(st.Config())

	// Find max field name length for alignment
	maxLen := len("ConfigPath")
	for i := 0; i < v.NumField(); i++ {
		name := t.Field(i).Name
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	// Get the path config is loaded from
	cpath := st.ConfigPath()

	// Would relative path give us a shorter filepath string?
	if wd, err := os.Getwd(); err == nil {
		ncpath, err := filepath.Rel(wd, cpath)
		if err == nil && len(ncpath) < len(cpath) {
			cpath = ncpath
		}
	}

	// Can we use '~' instead?
	if homeDir, err := os.UserHomeDir(); err == nil && strings.HasPrefix(cpath, homeDir) {
		ncpath := strings.Replace(cpath, homeDir, "~", 1)
		if len(ncpath) < len(cpath) {
			cpath = ncpath
		}
	}

	// Print the read-only environment path first
	fmt.Printf("  %-*s : %s\n", maxLen, "ConfigPath", cpath)

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if field.Name == "Alias" {
			continue
		}

		value := v.Field(i).Interface()
		fmt.Printf("  %-*s : %v\n", maxLen, field.Name, value)
	}

	fmt.Println("─────────────────────────────────────────────────────────────────")
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
// config save [<filepath>]
// config load <filepath>
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

	case "save":
		if nargs := len(args); nargs < 2 || nargs > 3 {
			fmt.Println("Usage: config save [<path>]")
			return
		}

		// If no save path mentioned, save to load path
		fpath := ctx.Store.ConfigPath()
		if len(args) == 3 {
			fpath = args[2]
		}

		dumpConfig(fpath, ctx.Store)

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

func buildConfigHelp() string {
	var buf bytes.Buffer
	buf.WriteString(`config [save [<path>] | load <path> | set <key> <val>]

Available Parameters:
`)

	fields := store.ConfigFields()

	// Compute max length for pretty printing
	maxLen := 0
	for _, f := range fields {
		maxLen = max(maxLen, len(f))
	}

	// Pretty print to buffer
	for _, f := range fields {
		desc := store.ConfigHelp[f]
		fmt.Fprintf(&buf, "  %-*s : %s\n", maxLen, f, desc)
	}

	return strings.TrimRight(buf.String(), "\n")
}

func init() {
	RegisterCommand(
		"config",
		handleConfig,
		"View, modify or save the current session configuration.",
		buildConfigHelp(),
		[]string{"save", "load", "set"}, // For autocompletion
	)
}
