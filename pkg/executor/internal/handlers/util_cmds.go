package script

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/store"
)

func updateVariable(args []string, st *store.Store) error {
	if len(args) != 2 && len(args) != 3 {
		return fmt.Errorf("syntax error, expected: `<incr|decr> <variable> [<value>]`")
	}

	// Parse the 2nd argument and resolve it
	key := strings.TrimSpace(args[1])
	val, ok, _ := st.Get("$VARS." + key)
	if !ok {
		return fmt.Errorf("variable not found: %s", key)
	}

	// Attempt to convert arg2 as a number
	raw := strings.TrimSpace(val)
	original, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("input value [%s] not a valid number: %w", raw, err)
	}

	// Parse the optional 3rd argument as a number
	delta := 1
	if len(args) == 3 {
		raw := strings.TrimSpace(args[2])
		res, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("update value [%s] not a valid number: %w", raw, err)
		}
		delta = res
	}

	// Perform the update based on 1st arg
	if strings.ToLower(args[0]) == "incr" {
		original += delta
	} else {
		original -= delta
	}

	// Always succeeds
	st.Set("$VARS."+key, strconv.Itoa(original))
	return nil
}

func handleIncr(ctx *ScriptContext, args []string) error {
	return updateVariable(args, ctx.Store)
}

func handleDecr(ctx *ScriptContext, args []string) error {
	return updateVariable(args, ctx.Store)
}

func handleSet(ctx *ScriptContext, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("syntax error, expected `set <key> <value>`")
	}

	key, value := strings.TrimSpace(args[1]), strings.TrimSpace(args[2])
	if _, _, err := ctx.Store.Set(key, value); err != nil {
		return fmt.Errorf("failed to set: %w", err)
	}

	return nil
}

func handlePrint(ctx *ScriptContext, args []string) error {
	fmt.Fprintln(ctx.Writer, strings.Join(args[1:], " "))
	return nil
}

func handleSleep(ctx *ScriptContext, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("syntax error, expected: `sleep <millis>`")
	}

	millis, err := strconv.Atoi(strings.TrimSpace(args[1]))
	if err != nil {
		return fmt.Errorf("not a valid integer: %w", err)
	}

	timer := time.NewTimer(time.Millisecond * time.Duration(millis))
	defer timer.Stop()

	// Wait until timer runs out or context is cancelled
	select {
	case <-ctx.GoCtx.Done():
		return nil
	case <-timer.C:
		return nil
	}
}

func compareAsFloat(arg1, arg2 string) (float64, float64, bool) {
	f1, err1 := strconv.ParseFloat(arg1, 64)
	f2, err2 := strconv.ParseFloat(arg2, 64)
	return f1, f2, err1 != nil && err2 != nil
}

func evaluateExpr[T cmp.Ordered](v1, v2 T, op string) bool {
	switch op {
	case "<":
		return v1 < v2
	case ">":
		return v1 > v2
	case "<=":
		return v1 <= v2
	case ">=":
		return v1 >= v2
	case "==":
		return v1 == v2
	case "!=":
		return v1 != v2
	default:
		return false
	}
}

// Just compare, already substituted before call
func handleAssert(_ *ScriptContext, args []string) error {
	if len(args) != 4 {
		return fmt.Errorf("syntax error, expected: `assert <expr1> <op> <expr2>`")
	}

	expr1, expr2 := strings.TrimSpace(args[1]), strings.TrimSpace(args[3])
	op := strings.TrimSpace(args[2])

	var result bool
	if f1, f2, numOk := compareAsFloat(expr1, expr2); numOk {
		result = evaluateExpr(f1, f2, op)
	} else {
		result = evaluateExpr(expr1, expr2, op)
	}

	if !result {
		return Falsy(fmt.Errorf("assert failed for expression: \"'%s' %s '%s'\"", expr1, op, expr2))
	}

	return nil
}

func init() {
	RegisterCommand("set", handleSet)       // set <key> <value>
	RegisterCommand("incr", handleIncr)     // incr <var> [<value>]
	RegisterCommand("decr", handleDecr)     // decr <var> [<value>]
	RegisterCommand("print", handlePrint)   // print [<value1>, [<value2> [...]]]
	RegisterCommand("sleep", handleSleep)   // sleep <millis>
	RegisterCommand("assert", handleAssert) // assert <expr1> <op> <expr2>
}
