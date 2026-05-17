package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/store"
)

// setupTestContext creates a dummy context for testing isolated commands
func setupTestContext() (*ScriptContext, *bytes.Buffer) {
	st := store.InitStore()
	buf := new(bytes.Buffer)

	ctx := &ScriptContext{
		GoCtx:   context.Background(),
		Session: nil,
		Store:   &st,
		Writer:  buf,
	}

	return ctx, buf
}

func TestEval_BasicCommand(t *testing.T) {
	ctx, buf := setupTestContext()

	// Register a dummy command just for this test
	RegisterCommand("testcmd", func(c *ScriptContext, args []string) error {
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
	ctx, _ := setupTestContext()

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

func TestUtilCmds_SetAndPrint(t *testing.T) {
	ctx, buf := setupTestContext()

	// Test Set
	if err := handleSet(ctx, []string{"set", "VARS.Price", "100.50"}); err != nil {
		t.Fatalf("handleSet failed: %v", err)
	}
	if val, ok, _ := ctx.Store.Get("VARS.Price"); !ok || val != "100.50" {
		t.Errorf("Store did not save value correctly, got: %v", val)
	}

	// Test Print (with variable substitution simulating Eval behavior)
	if err := handlePrint(ctx, []string{"print", "Price", "is", "100.50"}); err != nil {
		t.Fatalf("handlePrint failed: %v", err)
	}
	if expected := "Price is 100.50\n"; buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEval_Substitution(t *testing.T) {
	ctx, buf := setupTestContext()
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
