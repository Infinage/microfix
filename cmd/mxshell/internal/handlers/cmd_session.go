package shell

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/broker"
	"github.com/infinage/microfix/pkg/macros"
	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/pretty"
	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/store"
)

// Read from broker and write into circular buffer
func startLogger(lbroker *broker.Broker, cb *ringbuf.CircularBuffer, router spec.Router) error {
	// Subscribe to the log broker
	logCh, unsubscribe := lbroker.Subscribe()

	// Session closes logs on run loop exit
	go func() {
		// not required since closeAllLogs already cleans this up
		defer unsubscribe()
		var sb strings.Builder

		for log := range logCh {
			sb.Reset()
			pretty.Log(&sb, log, &router)
			cb.Write(strings.TrimSpace(sb.String()))
		}
	}()

	return nil
}

func NewSession(store *store.Store) (*session.Session, error) {
	// Create new session
	cfg := store.Config()
	return session.NewSession(
		cfg.SessionSpec,
		cfg.SenderCompID,
		cfg.TargetCompID,
		cfg.HeartbeatInt,
		session.EngineOptions{
			DefaultApplVer:   cfg.ApplicationSpec,
			SkipLatencyCheck: cfg.SkipLatencyCheckInValidate,
		},
	)
}

func handleStatus(ctx *ShellContext, _ []string) {
	snapshot := ctx.Session().Status()

	var stateColor string
	switch snapshot.State {
	case session.SessionActive:
		stateColor = "\033[32m" // Green
	case session.SessionLoggingIn, session.SessionStale, session.SessionOutOfSync:
		stateColor = "\033[33m" // Yellow
	default:
		stateColor = "\033[31m" // Red
	}

	// Calculate elapsed time for better UX
	now := time.Now()
	lastIn := "N/A"
	lastOut := "N/A"

	if !snapshot.LastReadTime.IsZero() {
		lastIn = fmt.Sprintf("%s ago", now.Sub(snapshot.LastReadTime).Round(time.Second))
	}
	if !snapshot.LastWriteTime.IsZero() {
		lastOut = fmt.Sprintf("%s ago", now.Sub(snapshot.LastWriteTime).Round(time.Second))
	}

	fmt.Println("\n─── Session Status ──────────────────────────────")
	fmt.Printf("  State      : %s%s\033[0m\n", stateColor, snapshot.State)
	fmt.Printf("  Sequence   : In(%d) | Out(%d)\n", snapshot.InSeqNum, snapshot.OutSeqNum)
	fmt.Printf("  Activity   : Last In: %s | Last Out: %s\n", lastIn, lastOut)
	fmt.Println("──────────────────────────────────────────────────")
}

func handleSend(ctx *ShellContext, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: send [-r] [-a] <FixString>")
		return
	}

	isRaw := false
	isAlias := false
	for _, a := range args {
		switch a {
		case "-r":
			isRaw = true

		case "-a":
			isAlias = true
		}
	}

	var msg message.Message
	var err error

	// Resolve from Alias registry
	raw := args[len(args)-1]
	if isAlias {
		if rawMsg, ok, _ := ctx.Store.Get("ALIAS." + raw); ok {
			raw = rawMsg
		} else {
			err = fmt.Errorf("Alias %v not found", raw)
		}
	}

	// Substitute placeholders if any (even if sending as 'raw')
	sess := ctx.Session()
	if err == nil {
		raw, err = macros.Substitute(raw, sess, ctx.Store)
	}

	// Parse the text into message struct
	if err == nil {
		delim := raw[len(raw)-1:]
		msg, err = message.MessageFromString(raw, delim)
	}

	fmt.Println("\n─── Send Message ────────────────────────────────")

	if err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	if err := ctx.Session().Send(msg, isRaw); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	fmt.Printf("  Status : OK\n")

	if msgType, ok := msg.Get(35); ok {
		fmt.Printf("  MsgType: %s\n", msgType)
	}

	if isRaw {
		fmt.Println("  Mode   : RAW")
	} else {
		fmt.Println("  Mode   : NORMAL")
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func handleConnect(ctx *ShellContext, args []string) {
	if len(args) > 2 {
		fmt.Println("Usage: connect [<host:port>]")
		return
	}

	// Even if user passes an invalid data, will fail at net.Dial
	cfg := ctx.Store.Config()
	addr := fmt.Sprintf("%s:%d", cfg.IpAddr, cfg.Port)
	if len(args) == 2 {
		addr = args[1]
	}

	fmt.Println("\n─── Connect ─────────────────────────────────────")

	if err := ctx.Session().Connect(addr); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	} else {
		startLogger(ctx.logBroker, ctx.Logs, *ctx.session.Router())
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  Remote : %s\n", addr)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func handleListen(ctx *ShellContext, args []string) {
	if len(args) > 2 {
		fmt.Println("Usage: listen [<host:port>]")
		return
	}

	// Even if user passes an invalid data, will fail at net.Dial
	cfg := ctx.Store.Config()
	addr := fmt.Sprintf("%s:%d", cfg.IpAddr, cfg.Port)
	if len(args) == 2 {
		addr = args[1]
	}

	fmt.Println("\n─── Listen ──────────────────────────────────────")

	if err := ctx.Session().Listen(addr); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	} else {
		startLogger(ctx.logBroker, ctx.Logs, *ctx.session.Router())
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  Remote : %s\n", addr)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func handleReset(ctx *ShellContext, _ []string) {
	fmt.Println("\n─── Session Reset ───────────────────────────────")

	if err := ctx.resetSession(); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("──────────────────────────────────────────────────")
		os.Exit(1)
	}

	fmt.Printf("  Status : OK\n")
	fmt.Println("  Info   : New session initialized")
	fmt.Println("──────────────────────────────────────────────────")
}

func handleSeq(ctx *ShellContext, args []string) {
	sess := ctx.Session()

	// View current sequence numbers
	if len(args) == 1 {
		snap := sess.Status()
		fmt.Println("\n─── Sequence Status ────────────────────────────")
		fmt.Printf("  InSeqNum  : %d\n", snap.InSeqNum)
		fmt.Printf("  OutSeqNum : %d\n", snap.OutSeqNum)
		fmt.Println("────────────────────────────────────────────────")
		return
	}

	sub := strings.ToLower(args[1])

	switch sub {
	case "in", "out":
		if len(args) != 3 {
			fmt.Println("Usage: seq [in|out] <seqNum>")
			return
		}

		seqNo, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil || seqNo <= 0 {
			fmt.Println("Invalid sequence number. Must be a positive integer.")
			return
		}

		snapshot := sess.Status()

		inSeq := snapshot.InSeqNum
		outSeq := snapshot.OutSeqNum

		field := ""
		if sub == "in" {
			inSeq = seqNo
			field = "InSeqNum"
		} else {
			outSeq = seqNo
			field = "OutSeqNum"
		}

		fmt.Println("\n─── Sequence Update ────────────────────────────")

		// Engine handles the state changes and emit the appropriate logs
		if err := sess.ResetSequence(inSeq, outSeq); err != nil {
			fmt.Printf("  Status : FAILED\n")
			fmt.Printf("  Error  : %v\n", err)
			fmt.Println("────────────────────────────────────────────────")
			return
		}

		fmt.Printf("  Status : OK\n")
		fmt.Printf("  Field  : %s\n", field)
		fmt.Printf("  Value  : %d\n", seqNo)
		fmt.Println("────────────────────────────────────────────────")

	default:
		fmt.Printf("Unknown seq subcommand: %s\n", sub)
	}
}

func handleHelp(ctx *ShellContext, args []string) {
	// If user asks: help <command>
	if len(args) > 1 {
		cmdName := args[1]
		cmd, ok := ShellCommandRegistry[cmdName]
		if !ok {
			fmt.Printf("Unknown command: %s\n", cmdName)
			return
		}

		fmt.Println("Command :", cmdName)
		fmt.Println("Desc    :", cmd.Description)
		fmt.Println("Usage   :", cmd.Usage)
		return
	}

	// Otherwise: list all commands
	fmt.Println("Available Commands:")
	fmt.Println("────────────────────────────────────────────")

	fmt.Println("\nMXShell - CLI FIX Client")
	fmt.Println("Author  : nj.deesa@gmail.com")
	fmt.Println("Version :", ctx.Version)
	fmt.Println("Commit  :", ctx.GitCommit)
	fmt.Println("")

	// Sort commands for stable output
	names := make([]string, 0, len(ShellCommandRegistry))
	for name := range ShellCommandRegistry {
		names = append(names, name)
	}
	sort.Strings(names)

	// Find max length for alignment
	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	// Print nicely aligned
	for _, name := range names {
		cmd := ShellCommandRegistry[name]
		fmt.Printf("  %-*s  │ %s\n", maxLen, name, cmd.Description)
	}

	fmt.Println("\nType 'help <command>' for more details.")
	fmt.Println("────────────────────────────────────────────")
}

func init() {
	RegisterCommand("status", handleStatus, "Display current session state and sequence numbers", "status", nil)
	RegisterCommand("connect", handleConnect, "Initiate a TCP connection to the target", "connect [<host:port>]", nil)
	RegisterCommand("listen", handleListen, "Listen on a local port for an incoming connection", "listen [<host:port>]", nil)
	RegisterCommand("reset", handleReset, "Close current session and initialize a new one", "reset", nil)
	RegisterCommand("seq", handleSeq, "View or manually override FIX sequence numbers", "seq [in|out] <SeqNum>", nil)

	RegisterCommand(
		"send", handleSend,
		"Send a FIX message to the remote target",
		"send [-r] [-a] <msg>",
		[]string{"-a", "-r"},
	)

	RegisterCommand(
		"help", handleHelp, "Display help", "help",
		[]string{"alias", "clear", "config", "connect", "disconnect", "fix",
			"help", "listen", "logs", "reset", "run", "send", "seq", "status"},
	)
}
