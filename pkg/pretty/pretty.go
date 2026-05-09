package pretty

import (
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

func FieldDef(w io.Writer, f spec.FieldDef) {
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

func SpecEntry(w io.Writer, e spec.Entry, lookup map[string]uint16, includeOptional bool, level int) {
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
			SpecEntry(w, child, lookup, includeOptional, level+1)
			fmt.Fprint(w, "\n")
		}
		fmt.Fprintf(w, "%s}", indent)
	}

	// Print one more new line for consistency with line 1 where we dont print it
	if level == 0 {
		fmt.Fprintln(w)
	}
}

// Pretty print the message input per spec
func Message(w io.Writer, msg *message.Message, ro *spec.Router) error {
	msgType, _ := msg.Get(35)
	sp := ro.SpecForMsgType(msgType)

	context, ok := sp.Messages[msgType]
	if !ok {
		printFields(w, msg, sp.Fields)
		return fmt.Errorf("unknown MsgType: '%s'", msgType)
	}

	// HEADER
	fmt.Fprintln(w, "[HEADER]")
	trees, pos := buildTrees(msg, sp, &sp.Header, 0)
	if len(trees) == 0 {
		fmt.Fprintln(w, "  (empty)")
	} else {
		printTrees(w, trees, 1)
	}

	// BODY
	fmt.Fprintln(w, "\n[BODY]")

	trees, pos = buildTrees(msg, sp, &context, pos)
	if len(trees) == 0 {
		fmt.Fprintln(w, "  (empty)")
	} else {
		printTrees(w, trees, 1)
	}

	// TRAILER
	fmt.Fprintln(w, "\n[TRAILER]")
	trees, pos = buildTrees(msg, sp, &sp.Trailer, pos)
	if len(trees) == 0 {
		fmt.Fprintln(w, "  (empty)")
	} else {
		printTrees(w, trees, 1)
	}

	if pos != len(*msg) {
		return fmt.Errorf("message does not fully conform to spec (parsed up to index %d)", pos)
	}

	return nil
}

// ------------------ Helpers ------------------

func getEnum(val string, fdef spec.FieldDef) (string, bool) {
	for _, enum := range fdef.Enums {
		if enum.Enum == val {
			return enum.Description, true
		}
	}
	return "", false
}

func printFields(w io.Writer, msg *message.Message, fields map[uint16]spec.FieldDef) {
	for _, f := range *msg {
		name, dtype := "UNKNOWN", ""
		enum := ""

		if fDef, ok := fields[f.Tag]; ok {
			name = fDef.Name
			dtype = fDef.Type

			if desc, ok := getEnum(f.Value, fDef); ok {
				enum = fmt.Sprintf("  → %s", desc)
			}
		}

		fmt.Fprintf(w, "  %-4d = %-25s %-22s%s\n",
			f.Tag,
			f.Value,
			fmt.Sprintf("%s (%s)", name, dtype),
			enum,
		)
	}
}

func printTrees(w io.Writer, trees []treeNode, level int) {
	for _, t := range trees {
		indent := strings.Repeat("  ", level)

		name := t.tagName
		if name == "" {
			name = "UNKNOWN"
		}

		line := fmt.Sprintf("%s %-4d = %-25s %-22s%s\n",
			indent,
			t.tag,
			t.value,
			fmt.Sprintf("%s (%s)", name, t.dtype),
			t.enumDescription,
		)

		fmt.Fprint(w, line)

		// Handle repeating groups with visual branches
		if len(t.children) > 0 {
			for i, child := range t.children {
				// Use a visual branch indicator for the group header
				fmt.Fprintf(w, "%s   └── Group %d\n", indent, i+1)
				// Increase indent for children
				printTrees(w, child, level+2)
			}
		}
	}
}

// ------------------ Tree Model ------------------

type treeNode struct {
	tag             uint16
	value           string
	tagName         string
	dtype           string
	enumDescription string
	children        [][]treeNode
}

// Build structured tree from FIX message
func buildTrees(msg *message.Message, sp *spec.Spec, ctx *spec.Entry, pos int) ([]treeNode, int) {
	// We have to clean up after we have processed one entry from group
	// to ensure we dont inadvertently consume entries belonging to another group
	localLookup := maps.Clone(ctx.Lookup)

	var result []treeNode
	for pos < len(*msg) {
		field := (*msg)[pos]
		fieldPos, inCtx := localLookup[field.Tag]

		// Unknown to context
		if !inCtx {
			// Completely unknown field → print anyway
			if _, ok := sp.Fields[field.Tag]; !ok {
				result = append(result, treeNode{
					tag:     field.Tag,
					value:   field.Value,
					tagName: "UNKNOWN",
					dtype:   "UNKNOWN",
				})
				pos++
				continue
			}

			// Known globally but not in this context → break (belongs to parent)
			// Ensure we break without updating position
			break
		}

		pos++ // Update only if we are in the right context
		nCtx := ctx.Entries[fieldPos]
		delete(localLookup, field.Tag)

		name, dtype, enum := nCtx.Name, "", ""

		if fDef, ok := sp.Fields[field.Tag]; ok {
			name = fDef.Name
			dtype = fDef.Type

			if desc, ok := getEnum(field.Value, fDef); ok {
				enum = fmt.Sprintf("  → %s", desc)
			}
		}

		node := treeNode{
			tag:             field.Tag,
			value:           field.Value,
			tagName:         name,
			dtype:           dtype,
			enumDescription: enum,
		}

		// Handle groups
		if nCtx.IsGroup {
			repeat := 1
			if v, err := field.AsInt(); err == nil {
				repeat = int(v)
			}

			for i := 0; i < repeat; i++ {
				children, newPos := buildTrees(msg, sp, &nCtx, pos)
				pos = newPos

				node.children = append(node.children, children)
			}
		}

		result = append(result, node)
	}

	return result, pos
}
