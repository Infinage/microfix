package script

import (
	"cmp"
	"fmt"
	"regexp"
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
	val, ok, _ := st.Get("VARS." + key)
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
	st.Set("VARS."+key, strconv.Itoa(original))
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

func handleIsSet(ctx *ScriptContext, args []string) error {
	for i := 1; i < len(args); i++ {
		key := strings.TrimSpace(args[i])
		if _, ok, _ := ctx.Store.Get(key); !ok {
			return Falsy(fmt.Errorf("key '%s' not set", key))
		}
	}
	return nil
}

func handleUnSet(ctx *ScriptContext, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("syntax error, expected `unset <key>`")
	}

	key := strings.TrimSpace(args[1])
	if _, _, err := ctx.Store.Unset(key); err != nil {
		return fmt.Errorf("failed to unset: %w", err)
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
	return f1, f2, err1 == nil && err2 == nil
}

func evaluateFuzzy(raw, pattern, op string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("not a valid regex: `%s`: %w", pattern, err)
	}

	switch matched := re.MatchString(raw); op {
	case "~":
		return matched, nil
	case "!~":
		return !matched, nil
	default:
		return false, fmt.Errorf("unexpected operator: %q", op)
	}
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
	if len(args) != 4 && len(args) != 3 {
		return fmt.Errorf("syntax error, expected: `assert <expr1> [<op>] <expr2>`")
	}

	op, expr1 := "==", strings.TrimSpace(args[1])
	var expr2 string
	if len(args) == 4 {
		op, expr2 = strings.TrimSpace(args[2]), strings.TrimSpace(args[3])
	} else {
		expr2 = strings.TrimSpace(args[2])
	}

	var err error
	var result bool
	if op == "~" || op == "!~" {
		result, err = evaluateFuzzy(expr1, expr2, op)
		if err != nil {
			return err // invalid regex
		}
	} else if f1, f2, numOk := compareAsFloat(expr1, expr2); numOk {
		result = evaluateExpr(f1, f2, op)
	} else {
		result = evaluateExpr(expr1, expr2, op)
	}

	if !result {
		return Falsy(fmt.Errorf("assert failed for expression: \"'%s' %s '%s'\"", expr1, op, expr2))
	}

	return nil
}

func handleLoadMsg(ctx *ScriptContext, args []string) error {
	typ := strings.TrimSpace(args[1])
	if len(args) != 3 || (typ != "in" && typ != "out") {
		return fmt.Errorf("syntax error, expected: `loadmsg <in|out> <msgId>`")
	}

	msgId := strings.TrimSpace(args[2])
	msg := ctx.Session().LastMessage(msgId, typ == "in")
	if msg == nil {
		return Falsy(fmt.Errorf("MsgId [%s] not found", msgId))
	}

	ctx.Store.SetBuffer(*msg)
	return nil
}

func init() {
	RegisterCommand("set", handleSet)         // set <key> <value>
	RegisterCommand("isset", handleIsSet)     // isset [<key1>, [<key2> [...]]]
	RegisterCommand("unset", handleUnSet)     // unset <key>
	RegisterCommand("incr", handleIncr)       // incr <var> [<value>]
	RegisterCommand("decr", handleDecr)       // decr <var> [<value>]
	RegisterCommand("print", handlePrint)     // print [<value1>, [<value2> [...]]]
	RegisterCommand("sleep", handleSleep)     // sleep <millis>
	RegisterCommand("assert", handleAssert)   // assert <expr1> <op> <expr2>
	RegisterCommand("loadmsg", handleLoadMsg) // loadmsg <in|out> <msgId>
}
