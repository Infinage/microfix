package executor

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/infinage/microfix/pkg/macros"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"

	"github.com/infinage/microfix/pkg/executor/internal/handlers"
)

const ScriptHelpText = `
Scripts are executed line-by-line. Arguments are separated by spaces.
Use quotes (" ") to group arguments containing spaces.
Lines starting with '#' are ignored as comments.

SESSION COMMANDS
  connect [<host:port>]     Connect to target (defaults to config if omitted)
  listen [<host:port>]      Listen for incoming connection (defaults to config)
  disconnect                Close the active connection
  reset                     Close and re-initialize a fresh session
  seq <in|out> <SeqNum>     Manually override the inbound or outbound sequence number

MESSAGING COMMANDS
  send [-r] <msg>           Send a FIX message. Use -r to send raw (skip validation)
  wait <MsgLike>            Block and wait until a message matching <MsgLike> is received
  expect <MsgLike>          Fail if the *next* app message doesn't match <MsgLike>
                            (Automatically ignores background Heartbeats & Test Requests)
  loadmsg <in|out> <id>     Load a specific message from session history into the buffer

MSGLIKE SYNTAX
  A MsgLike is a boolean expression over FIX tags.

  35=D                      Tag 35 equals D
  35=D & 11=ABC             AND
  35=D | 35=G               OR
  !39=4                     NOT
  (35=D | 35=G) & !39=4     Grouping with parentheses

  Operator precedence: !, then &, then |

CONTROL FLOW
  if <cmd>                  Execute block if <cmd> succeeds (e.g., if assert 1 == 1)
  elif <cmd>                Execute block if previous conditions failed and <cmd> succeeds
  else                      Execute block if all previous conditions failed
  endif                     Close an if/elif/else block
  while <cmd>               Loop block as long as <cmd> succeeds
  endwhile                  Close a while loop
  break                     Exit the current while loop early
  continue                  Skip to the next iteration of the current while loop
  exit                      Immediately terminate script execution

SCRIPT FLOW & UTILITY
  set <key> <val>           Set a variable in the store (e.g., set VARS.Symbol AAPL)
  unset <key>               Remove a variable from the store (e.g., unset VARS.Symbol)
  isset <key1> [...]        Succeeds only if all specified keys exist in the store
  incr <key> [<val>]        Increment a numeric variable by 1 (or by optional <val>)
  decr <key> [<val>]        Decrement a numeric variable by 1 (or by optional <val>)
  print <val> [...]         Print text or variables to the console
  sleep <millis>            Pause execution for N milliseconds
  include <path>            Include and execute another script file
  assert <e1> [<op>] <e2>   Fail script if expression is false.
                            Ops: ==, !=, >, <, >=, <=, ~, !~
  waitstatus <state>        Block until session enters state (New, Listening,
                            LoggingIn, Active, Stale, OutOfSync, Closed)

GLOBAL VARIABLES
  Variables can be injected into any command using the '$' prefix.

  -- System & State --
  $UNIQUE                   Random UUID (e.g., for ClOrdID generation)
  $TIMESTAMP                Current UTC timestamp (YYYYMMDD-HH:MM:SS.000)
  $DATE                     Current date (YYYYMMDD)
  $DATE[+N]                 Date offset by N days (e.g., $DATE[+1] is tomorrow)
  $STATUS                   Current session state (e.g., "Active", "Closed")
  $SEQ_IN / $SEQ_OUT        Current internal Inbound/Outbound Sequence Number

  -- Context & Store --
  $CFG.<key>                Config values
  $VARS.<key>               Script-defined values (set via 'set' command)
  $ALIAS.<name>             Saved aliases
  $ENV.<name>               Environment variables
  $BUF.<tag>                Extract integer <tag> from the currently buffered message.
                            Message is loaded into buffer upon a successful 'wait',
                            'expect', or explicit 'loadmsg'. Will fail if buffer is empty.
                            (e.g., $BUF.35 or $BUF.11)
  $LASTIN[T,t]              Extract tag 't' from last incoming message of MsgType 'T'
  $LASTOUT[T,t]             Extract tag 't' from last outgoing message of MsgType 'T'
                            (e.g., $LASTOUT[8,39] gets OrdStatus from ExecutionReport)
`

func NewScriptContext(
	getSession func() *session.Session,
	resetSession func() error,
	st *store.Store,
	writer io.Writer,
) (script.ScriptContext, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return script.ScriptContext{
		GoCtx:   ctx,
		Session: getSession,
		Reset:   resetSession,
		Store:   st,
		Writer:  writer,
	}, cancel
}

// Evaluate a single line with provided context
func Eval(line string, ctx *script.ScriptContext) error {
	expandedLine, err := macros.Substitute(line, ctx.Session(), ctx.Store, true)
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
	instructions, jumpTable, err := parseJumpTable(r)
	if err != nil {
		return fmt.Errorf("failed to compile script: %w", err)
	}

	var justJumped bool
	for pc := 0; pc < len(instructions); {
		// Check context cancellation
		select {
		case <-ctx.GoCtx.Done():
			return fmt.Errorf("script cancelled")

		default:
		}

		// Ensure jump table exists for required commands
		instr := instructions[pc]
		jump, ok := jumpTable[pc]
		if instr.Type != "" && instr.Type != "endif" && instr.Type != "exit" && !ok {
			return fmt.Errorf("jump entry not found for [%s], line# %d", instr.Type, instr.LineNo)
		}

		// Reached sequentially (!justJumped) => jump to TargetOnEnd
		if (instr.Type == "elif" || instr.Type == "else" || instr.Type == "endwhile") && !justJumped {
			pc = jump.TargetOnEnd
			continue
		}

		// jumps already handled, reset flag
		justJumped = false

		switch instr.Type {
		case "exit":
			return nil

		// justJumped was true => do nothing (incr pc)
		case "endif", "else", "endwhile":

		// Unconditionally jump to TargetOnEnd
		case "break", "continue":
			pc = jump.TargetOnEnd
			justJumped = true
			continue

		// If evaluates to true  => continue into block (incr pc)
		// If evaluates to falsy => jump to TargetOnFalse
		// If evaluates as error => return as error
		case "if", "elif", "while":
			err := Eval(instr.Text, ctx)
			if _, isFalsy := errors.AsType[*script.FalsyError](err); isFalsy {
				pc = jump.TargetOnFalse
				justJumped = true
				continue
			} else if err != nil {
				return fmt.Errorf("line %d: %w", instr.LineNo, err)
			}

		// Normal commands
		default:
			if err := Eval(instr.Text, ctx); err != nil {
				return fmt.Errorf("line %d: %w", instr.LineNo, err)
			}
		}

		pc++ // sequential execution
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
