package executor

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/infinage/microfix/pkg/executor/internal/handlers"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

func NewScriptContext(sess *session.Session, st *store.Store, writer io.Writer) (script.ScriptContext, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return script.ScriptContext{GoCtx: ctx, Session: sess, Store: st, Writer: writer}, cancel
}

// Evaluate a single line with provided context
func Eval(line string, ctx *script.ScriptContext) error {
	expandedLine, err := Substitute(line, ctx)
	if err != nil {
		return fmt.Errorf("substitution failed: %w", err)
	}

	creader := csv.NewReader(strings.NewReader(expandedLine))
	creader.Comma = ' '

	args, err := creader.Read()
	if err != nil || len(args) == 0 {
		return fmt.Errorf("not a valid input: %w", err)
	}

	cmdName := strings.ToLower(strings.TrimSpace(args[0]))
	cmdHandler, ok := script.ScriptRegistry[cmdName]
	if !ok {
		return fmt.Errorf("unknown command: %v", cmdName)
	}

	if err := cmdHandler(ctx, args); err != nil {
		return fmt.Errorf("execute failed for '%v': %w", strings.Join(args, " "), err)
	}

	return nil
}

// Evaluate from input source line by line, passing each result to callback provided
func EvalBatch(r io.Reader, ctx *script.ScriptContext) error {
	lineNo, reader := 0, bufio.NewReader(r)
	for {
		// Read a single line
		line, err := reader.ReadString('\n')
		lineNo++ // Starts at #1

		// Return on error, if we hit on EOF, it maybe that file ended
		// without '\n', we want to process line and exit after processing it
		if err != nil && err != io.EOF {
			return err
		}

		// Ignore empty lines and comments
		line = strings.TrimSpace(line)
		if line != "" && line[0] != '#' {
			// Evaluate the line and exit early on error
			if err := Eval(line, ctx); err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
		}

		// Break on EOF
		if err == io.EOF {
			break
		}
	}

	return nil
}

func handleInclude(ctx *script.ScriptContext, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("syntax error, usage: `include <filepath>`")
	}

	f, err := os.Open(strings.TrimSpace(args[1]))
	if err != nil {
		return fmt.Errorf("failed to read: %w", err)
	}

	defer f.Close()
	return EvalBatch(f, ctx)
}

func init() {
	script.RegisterCommand("include", handleInclude) // include <file>
}
