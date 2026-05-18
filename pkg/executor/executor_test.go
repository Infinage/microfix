package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	script "github.com/infinage/microfix/pkg/executor/handlers"
	"github.com/infinage/microfix/pkg/store"
)

// setupTestContext creates a dummy context for testing isolated commands
func setupTestContext(t *testing.T) (*script.ScriptContext, *bytes.Buffer) {
	t.Helper()

	st := store.InitStore()
	buf := new(bytes.Buffer)

	ctx := &script.ScriptContext{
		GoCtx:   context.Background(),
		Session: nil,
		Store:   &st,
		Writer:  buf,
	}

	return ctx, buf
}

func TestEval_BasicCommand(t *testing.T) {
	ctx, buf := setupTestContext(t)

	// Register a dummy command just for this test
	script.RegisterCommand("testcmd", func(c *script.ScriptContext, args []string) error {
		c.Writer.Write([]byte("executed " + args[1]))
		return nil
	})

	err := Eval("testcmd hello", ctx)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	if buf.String() != "executed hello" {
		t.Errorf("Expected 'executed hello', got '%s'", buf.String())
	}
}

func TestEvalBatch_CommentsAndEmptyLines(t *testing.T) {
	ctx, _ := setupTestContext(t)

	script := `
# This is a comment
   # Indented comment

set VARS.Symbol AAPL
print $VARS.Symbol
`
	if err := EvalBatch(strings.NewReader(script), ctx); err != nil {
		t.Fatalf("EvalBatch failed: %v", err)
	}

	if val, ok, _ := ctx.Store.Get("VARS.Symbol"); !ok || val != "AAPL" {
		t.Errorf("Expected store to have AAPL, got %v", val)
	}
}

func TestEval_Substitution(t *testing.T) {
	ctx, buf := setupTestContext(t)
	ctx.Store.Set("VARS.Target", "MOCK_EXCHANGE")

	err := Eval("print Connecting to $VARS.Target", ctx)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	expected := "Connecting to MOCK_EXCHANGE\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}
