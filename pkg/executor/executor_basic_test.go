package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	script "github.com/infinage/microfix/pkg/executor/internal/handlers"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

// setupTestContext creates a dummy context for testing isolated commands
func setupTestContext(t *testing.T, sess *session.Session) (*script.ScriptContext, *bytes.Buffer) {
	t.Helper()

	st := store.InitStore()
	buf := new(bytes.Buffer)

	ctx := &script.ScriptContext{
		GoCtx:   context.Background(),
		Session: func() *session.Session { return sess },
		Store:   &st,
		Writer:  buf,
	}

	return ctx, buf
}

func TestEval_BasicCommand(t *testing.T) {
	ctx, buf := setupTestContext(t, nil)

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
	ctx, _ := setupTestContext(t, nil)

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
	ctx, buf := setupTestContext(t, nil)
	ctx.Store.Set("VARS.Target", "MOCK_EXCHANGE")

	if err := Eval("print Connecting to $VARS.Target", ctx); err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	if expected := "Connecting to MOCK_EXCHANGE\n"; buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}

	t.Run("Testing assert command", func(t *testing.T) {
		ctx.Store.Set("VARS.Target", "MOCK_EXCHANGE")
		if err := Eval("assert $VARS.Target MOCK_EXCHANGE", ctx); err != nil {
			t.Errorf("Expected assertion to pass, failed: %v", err)
		}
		if err := Eval("assert MOCK_EXCHANGE $VARS.Target", ctx); err != nil {
			t.Errorf("Expected assertion to pass, but failed: %v", err)
		}

		ctx.Store.Set("VARS.Target", "0")
		if err := Eval("assert MOCK_EXCHANGE $VARS.Target", ctx); err == nil {
			t.Error("Expected assertion to fail, but passed")
		}
	})
}

func TestEvalBatch_ControlFlow(t *testing.T) {
	// Define test cases
	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name: "Simple If - True",
			script: `
				if assert 1 == 1
					print YES
				endif
			`,
			expected: "YES\n",
		},
		{
			name: "Simple If - False",
			script: `
				if assert 1 == 2
					print YES
				endif
			`,
			expected: "",
		},
		{
			name: "If Else - Falls to Else",
			script: `
				if assert 1 == 2
					print NO
				else
					print YES
				endif
			`,
			expected: "YES\n",
		},
		{
			name: "If Elif Else - Elif matches",
			script: `
				if assert 1 == 2
					print 1
				elif assert 1 == 1
					print 2
				else
					print 3
				endif
			`,
			expected: "2\n",
		},
		{
			name: "If Elif Else - Skips Elif when If matches",
			script: `
				if assert 1 == 1
					print 1
				elif assert 1 == 1
					print 2
				else
					print 3
				endif
			`,
			expected: "1\n",
		},
		{
			name: "While Loop",
			script: `
				set VARS.loop 3
				while assert $VARS.loop > 0
					print ITER
					decr loop
				endwhile
			`,
			expected: "ITER\nITER\nITER\n",
		},
		{
			name: "While Loop with Break",
			script: `
				set VARS.loop 3
				while assert $VARS.loop > 0
					print ITER
					if assert $VARS.loop == 3
						break
					endif
					decr loop
				endwhile
			`,
			expected: "ITER\n",
		},
		{
			name: "Nested Logic",
			script: `
				set VARS.loop 3
				while assert $VARS.loop > 0
					if assert 1 == 2
						print FAIL
					else
						print PASS
					endif
					decr loop
				endwhile
			`,
			expected: "PASS\nPASS\nPASS\n",
		},
	}

	// Run the tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, buf := setupTestContext(t, nil)

			if err := EvalBatch(strings.NewReader(tt.script), ctx); err != nil {
				t.Fatalf("EvalBatch failed: %v", err)
			}

			if got := buf.String(); got != tt.expected {
				t.Errorf("\nExpected:\n%q\nGot:\n%q", tt.expected, got)
			}
		})
	}
}
