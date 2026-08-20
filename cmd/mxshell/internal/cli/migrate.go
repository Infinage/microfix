package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/infinage/microfix/pkg/migrate"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/store"
)

func ExtractFromMicroFIX(fpath string) {
	// Pick config from home or cwd, else init defaults
	st := store.InitStore()
	sessSp, applSp := st.Config().SessionSpec, st.Config().ApplicationSpec
	if applSp == "" {
		applSp = sessSp
	}

	// Load router from provided config, need to 'salvage' message
	ro, err := spec.NewRouter(sessSp, []string{applSp})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load router: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Open(fpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open file %q: %v\n", fpath, err)
		os.Exit(1)
	}
	defer f.Close()

	aliasMsg, failed, err := migrate.ExtractAliasFromMiniFIX(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: extraction failed: %v\n", err)
		os.Exit(1)
	}

	alias := make(map[string]string, len(aliasMsg))
	for name, msg := range aliasMsg {
		msg := ro.Salvage(msg)
		alias[name] = msg.String("\x01")
	}

	jsonRes, err := json.MarshalIndent(alias, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to format output: %v\n", err)
		os.Exit(1)
	}

	// Log failures as warning
	for _, name := range failed {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse alias %q\n", name)
	}

	// Print the count + actual JSON payload to stdout
	fmt.Fprintf(os.Stderr, "Successfully extracted %d/%d aliases.\n", len(alias), len(alias)+len(failed))
	fmt.Println(string(jsonRes))
}
