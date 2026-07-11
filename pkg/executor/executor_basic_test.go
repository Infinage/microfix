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
		{
			name: "While Loop with Continue",
			script: `
				set VARS.loop 3
				while assert $VARS.loop > 0
					decr loop
					if assert $VARS.loop == 1
						continue
					endif
					print ITER $VARS.loop
				endwhile
			`,
			expected: "ITER 2\nITER 0\n",
		},
		{
			name: "Nested While Loops with Break",
			script: `
				set VARS.outer 2
				while assert $VARS.outer > 0
					set VARS.inner 2
					while assert $VARS.inner > 0
						if assert $VARS.inner == 1
							break
						endif
						print INNER $VARS.inner
						decr inner
					endwhile
					print OUTER $VARS.outer
					decr outer
				endwhile
			`,
			expected: "INNER 2\nOUTER 2\nINNER 2\nOUTER 1\n",
		},
		{
			name: "Nested While Loops with Continue",
			script: `
				set VARS.outer 2
				while assert $VARS.outer > 0
					set VARS.inner 2
					while assert $VARS.inner > 0
						decr inner
						if assert 1 == 1
							continue
						endif
						print SHOULD_NOT_SEE
					endwhile
					print OUTER $VARS.outer
					decr outer
				endwhile
			`,
			expected: "OUTER 2\nOUTER 1\n",
		},
		{
			name: "Falsy Error Storage - If/Else",
			script: `
                if assert 1 == 2
                    print SHOULD_NOT_RUN
                else
                    print $ERROR
                endif
            `,
			expected: "assertion failed: '1 == 2'\n",
		},
		{
			name: "Falsy Error Storage - Elif Chain",
			script: `
                if assert AAPL == MSFT
                    print ONE
                elif assert 50 > 100
                    print TWO
                else
                    print $ERROR
                endif
            `,
			expected: "assertion failed: '50 > 100'\n",
		},
		{
			name: "Falsy Error Cleared/Not present initially",
			script: `
                if assert 1 == 1
                    print NO_ERROR_$ERROR
                endif
            `,
			expected: "NO_ERROR_\n",
		},
		{
			name: "Not Command - Inverts True to Falsy",
			script: `
                if not assert 1 == 1
                    print SHOULD_NOT_RUN
                endif
				print $ERROR
            `,
			expected: "assertion failed: 'not assert 1 == 1'\n",
		},
		{
			name: "Not Command - Inverts Falsy to True",
			script: `
                if not assert 1 == 2
                    print INVERTED_SUCCESS
                endif
            `,
			expected: "INVERTED_SUCCESS\n",
		},
		{
			name: "Standalone Not captures FalsyError without interrupting flow",
			script: `
				not assert 1 2
				print $ERROR
			`,
			expected: "assertion failed: '1 == 2'\n",
		},
		{
			name: "Control flow overrides NOT's captures",
			script: `
				if not assert 1 2
				endif
				print $ERROR
			`,
			expected: "\n",
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

func TestEval_IssetUnset(t *testing.T) {
	ctx, _ := setupTestContext(t, nil)

	// Test isset on a missing variable
	if err := Eval("isset VARS.Missing", ctx); err == nil {
		t.Error("Expected isset to fail for missing variable, but it passed")
	}
	if err := Eval("not isset VARS.Missing", ctx); err != nil {
		t.Errorf("Expected 'not isset' to pass for missing variable, got error: %v", err)
	}

	// Set variable and test isset (Should pass)
	if err := Eval("set VARS.Found 123", ctx); err != nil {
		t.Fatalf("Failed to set variable: %v", err)
	}
	if err := Eval("isset VARS.Found", ctx); err != nil {
		t.Errorf("Expected isset to pass for existing variable, got error: %v", err)
	}
	if err := Eval("not isset VARS.Found", ctx); err == nil {
		t.Error("Expected 'not isset' to fail for existing variable, but it passed")
	}

	// Test multi-isset (Should pass when all exist)
	Eval("set VARS.Second 456", ctx)
	if err := Eval("isset VARS.Found VARS.Second", ctx); err != nil {
		t.Errorf("Expected multi-isset to pass, got error: %v", err)
	}

	// Unset one and test multi-isset again (Should fail)
	if err := Eval("unset VARS.Found", ctx); err != nil {
		t.Fatalf("Failed to unset variable: %v", err)
	}
	if err := Eval("isset VARS.Found VARS.Second", ctx); err == nil {
		t.Error("Expected multi-isset to fail after unsetting one variable, but it passed")
	}
}

func TestEval_AssertRegex(t *testing.T) {
	ctx, _ := setupTestContext(t, nil)
	ctx.Store.Set("VARS.LogMsg", "ExecutionReport: Filled")

	// Test Regex Prefix Match (~)
	if err := Eval("assert $VARS.LogMsg ~ ^ExecutionReport.*", ctx); err != nil {
		t.Errorf("Expected regex prefix match to pass, got: %v", err)
	}

	// Test Regex Suffix Match (~)
	if err := Eval("assert $VARS.LogMsg ~ .*Filled$", ctx); err != nil {
		t.Errorf("Expected regex suffix match to pass, got: %v", err)
	}

	// Test Regex Negative Match (!~)
	if err := Eval("assert $VARS.LogMsg !~ ^OrderSingle.*", ctx); err != nil {
		t.Errorf("Expected negative regex match (!~) to pass, got: %v", err)
	}

	// Test Failing Regex Match
	if err := Eval("assert $VARS.LogMsg ~ ^OrderSingle.*", ctx); err == nil {
		t.Error("Expected regex match to fail for incorrect pattern, but it passed")
	}

	// Test Invalid Regex Pattern
	if err := Eval("assert string ~ [broken-regex", ctx); err == nil {
		t.Error("Expected invalid regex compilation to fail gracefully, but it passed")
	}
}
