package handlers

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/infinage/microfix/internal/mxshell/config"
)

func printConfig(cfg *config.Config) {
	fmt.Println("\n─── Configuration ────────────────────────────────")

	t := reflect.TypeFor[config.Config]()
	v := reflect.ValueOf(*cfg)

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
		value := v.Field(i).Interface()

		fmt.Printf("  %-*s : %v\n", maxLen, field.Name, value)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func loadConfig(filename string, cfg *config.Config) {
	fmt.Println("\n─── Config Load ──────────────────────────────────")

	if newCfg, err := config.LoadConfig(filename); err == nil {
		*cfg = *newCfg
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  File   : %s\n", filename)
	} else {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func dumpConfig(filepath string, cfg *config.Config) {
	fmt.Println("\n─── Config Save ──────────────────────────────────")

	if err := cfg.Dump(filepath); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	} else {
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  File   : %s\n", filepath)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func setConfig(cfg *config.Config, key string, value string) {
	fmt.Println("\n─── Config Update ────────────────────────────────")

	v := reflect.ValueOf(cfg).Elem()
	field := v.FieldByName(key)

	if !field.IsValid() || !field.CanSet() {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : Field '%s' not found\n", key)
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	oldVal := fmt.Sprint(field.Interface())

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int64:
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			field.SetInt(i)
		} else {
			fmt.Printf("  Status : FAILED\n")
			fmt.Printf("  Error  : Invalid integer '%s'\n", value)
			return
		}

	case reflect.Uint16:
		if u, err := strconv.ParseUint(value, 10, 16); err == nil {
			field.SetUint(u)
		} else {
			fmt.Printf("  Status : FAILED\n")
			fmt.Printf("  Error  : Invalid port '%s'\n", value)
			return
		}

	case reflect.Bool:
		if b, err := strconv.ParseBool(value); err == nil {
			field.SetBool(b)
		} else {
			fmt.Printf("  Status : FAILED\n")
			fmt.Printf("  Error  : Invalid boolean '%s'\n", value)
			return
		}

	default:
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : Unsupported type %s\n", field.Kind())
		return
	}

	fmt.Printf("  Status : OK\n")
	fmt.Printf("  Field  : %s\n", key)
	fmt.Printf("  Old    : %s\n", oldVal)
	fmt.Printf("  New    : %v\n", field.Interface())

	fmt.Println("──────────────────────────────────────────────────")
}

// config - list all configs
// config [load|save] <filepath>
// config set <field> <value>
func handleConfig(ctx *AppContext, args []string) {
	if len(args) == 1 {
		printConfig(ctx.Config)
		return
	}

	sub := strings.ToLower(args[1])
	switch sub {
	case "load":
		if len(args) < 3 {
			fmt.Println("Usage: config load <path>")
			return
		}
		loadConfig(args[2], ctx.Config)

	case "save":
		path := ".mxrc"
		if len(args) > 2 {
			path = args[2]
		}
		dumpConfig(path, ctx.Config)

	case "set":
		if len(args) != 4 {
			fmt.Println("Usage: config set <key> <value>")
			return
		}
		setConfig(ctx.Config, args[2], args[3])

	default:
		fmt.Printf("Unknown config subcommand: %s\n", sub)
	}
}

func init() {
	RegisterCommand(
		"config",
		handleConfig,
		"View or modify the current session configuration",
		"config [load <path> | save <path> | set <key> <val>]",
	)
}
