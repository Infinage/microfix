package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/infinage/microfix/pkg/migrate"
)

func ExtractFromMicroFIX(fpath string) {
	f, err := os.Open(fpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open file %q: %v\n", fpath, err)
		os.Exit(1)
	}
	defer f.Close()

	alias, err := migrate.ExtractAliasFromMiniFIX(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: extraction failed: %v\n", err)
		os.Exit(1)
	}

	jsonRes, err := json.MarshalIndent(alias, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to format output: %v\n", err)
		os.Exit(1)
	}

	// Print the count + actual JSON payload to stdout
	fmt.Fprintf(os.Stderr, "Successfully extracted %d aliases.\n", len(alias))
	fmt.Println(string(jsonRes))
}
