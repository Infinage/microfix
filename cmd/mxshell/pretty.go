package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/infinage/microfix/pkg/spec"
)

func WritePrettyFieldDef(w io.Writer, f spec.FieldDef) {
	// Write the Header
	fmt.Fprintf(w, "Field: %s\nType:  %s\n", f.Name, f.Type)

	// Write the Enums if they exist
	if len(f.Enums) > 0 {
		fmt.Fprint(w, "\nValues:\n")
		for _, en := range f.Enums {
			// Using %-5s to give the enum code a fixed width for alignment
			fmt.Fprintf(w, "  %-5s -> %s\n", en.Enum, en.Description)
		}
	}
}

func WritePrettySpecEntry(w io.Writer, e spec.Entry, lookup map[string]uint16, includeOptional bool, level int) {
	indent := strings.Repeat("  ", level)

	// Build Indicators
	group := ""
	if e.IsGroup {
		group = "[G] "
	}
	req := ""
	if e.Required {
		req = "*"
	}

	// Format and Write the current line
	tag := lookup[e.Name]
	if level > 0 {
		fmt.Fprintf(w, "%s%s%s (%d)%s", indent, group, e.Name, tag, req)
	}

	// Recurse for Children
	if len(e.Entries) > 0 {
		fmt.Fprintf(w, "\n%s{\n", indent)
		for _, child := range e.Entries {
			// Filter out optional fields based on config
			if !includeOptional && !child.Required {
				continue
			}
			WritePrettySpecEntry(w, child, lookup, includeOptional, level+1)
			fmt.Fprint(w, "\n")
		}
		fmt.Fprintf(w, "%s}", indent)
	}

	// Print one more new line for consistency with line 1 where we dont print it
	if level == 0 {
		fmt.Fprintln(w)
	}
}
