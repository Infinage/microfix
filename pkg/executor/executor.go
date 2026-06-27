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

const ScriptHelpText = `
Scripts are executed line-by-line. Arguments are separated by spaces.
Use quotes (" ") to group arguments containing spaces.
Lines starting with '#' are ignored as comments.

SESSION COMMANDS
  connect [<host:port>]   Connect to target (defaults to config if omitted)
  listen [<host:port>]    Listen for incoming connection (defaults to config)
  disconnect              Close the active connection
  reset                   Close and re-initialize a fresh session
  seq <in|out> <SeqNum>   Manually override the inbound or outbound sequence number

MESSAGING COMMANDS
  send [-r] <msg>         Send a FIX message. Use -r to send raw (skip validation)
  expect <MsgLike>        Fail if the *next* app message doesn't match <MsgLike>
                          (Automatically ignores background Heartbeats & Test Requests)
  wait <MsgLike>          Block and wait until a message matching <MsgLike> is received

SCRIPT FLOW & UTILITY
  set <key> <val>         Set a variable in the store (e.g., set VARS.Symbol AAPL)
  print <val> [...]       Print text or variables to the console
  sleep <millis>          Pause execution for N milliseconds
  assert <exp1> <exp2>    Fail the script if exp1 != exp2
  include <filepath>      Include and execute another script file
  waitstatus <state>      Block until session enters state (New, Listening, 
                          LoggingIn, Active, Stale, OutOfSync, Closed)

VARIABLES & SUBSTITUTION
  Variables can be injected into any command using the '$' prefix.

  Store Vars:     $VARS.<name>, $CFG.<name>, $ALIAS.<name>, $ENV.<name>
  $UNIQUE         Generates a random UUID (e.g., for ClOrdID generation)
  $TIMESTAMP      Current UTC timestamp (YYYYMMDD-HH:MM:SS.000)
  $DATE           Current date (YYYYMMDD)
  $DATE[+N]       Date offset by N days (e.g., $DATE[+1] is tomorrow, $DATE[-1] is yesterday)
  $SEQ_IN         Current internal Inbound Sequence Number
  $SEQ_OUT        Current internal Outbound Sequence Number
  $STATUS         Current session state (e.g., "Active", "LoggingIn")
  $LASTIN[T,t]    Extract tag 't' from the last incoming message of MsgType 'T'
                  (e.g., $LASTIN[8,39] gets OrdStatus from the last ExecutionReport)
  $LASTOUT[T,t]   Extract tag 't' from the last outgoing message of MsgType 'T'
`

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
