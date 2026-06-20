package cli

import "fmt"

const scriptHelpText = `MXShell Scripting Reference

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

func PrintHelp(args []string) {
	// Print general help
	fmt.Print(
		"MXShell — FIX CLI Client\n\n" +
			"Usage:\n" +
			"  mxshell                  Start interactive shell\n" +
			"  mxshell -f <file> [-v]   Execute script in headless mode (-v for verbose logs)\n" +
			"  mxshell -h               Display help\n\n")

	// Detailed help with script syntax document
	if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
		fmt.Print("----------------------------------------------------------\n\n" + scriptHelpText)
	}
}
