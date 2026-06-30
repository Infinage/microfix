package script

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/store"
)

// <connect|listen> [<host:port>]
func parseStartSession(args []string, cfg *store.Config) (string, uint16, error) {
	// Connect from Config defaults
	if len(args) == 1 {
		return "", 0, nil
	}

	if len(args) > 2 {
		return "", 0, fmt.Errorf("invalid syntax, expected: `<connect|listen> [<host:port>]`")
	}

	splits := strings.SplitN(args[1], ":", 2)
	if len(splits) != 2 {
		return "", 0, fmt.Errorf("invalid format, expected: `host:port` as second arg")
	}

	port64, err := strconv.ParseUint(strings.TrimSpace(splits[1]), 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %w", err)
	}

	// Final variables from input passed
	host, port := strings.TrimSpace(splits[0]), uint16(port64)

	// If missing, fill in from configs
	if host == "" {
		host = cfg.IpAddr
	}
	if port == 0 {
		port = cfg.Port
	}

	return host, port, nil
}

// send [-r] <msg>
func parseSend(args []string) (string, bool, error) {
	nargs := len(args)
	if nargs < 2 || nargs > 3 || (nargs == 3 && strings.TrimSpace(args[1]) != "-r") {
		return "", false, fmt.Errorf("Usage: send [-r] <FixString>")
	}

	if nargs == 2 {
		return strings.TrimSpace(args[1]), false, nil
	}

	return strings.TrimSpace(args[2]), true, nil
}

// seq <in|out> <SeqNum>
func parseResetSequence(args []string) (int64, bool, error) {
	if len(args) != 3 {
		return 0, false, fmt.Errorf("invalid syntax, expected: `seq <in|out> <SeqNum>`")
	}

	subCmd := strings.TrimSpace(args[1])
	if subCmd != "in" && subCmd != "out" {
		return 0, false, fmt.Errorf("unknown subcommand, must be one of 'in' or 'out'")
	}

	seqNo, err := strconv.ParseInt(strings.TrimSpace(args[2]), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid SeqNo provided: %w", err)
	}

	return seqNo, subCmd == "in", nil
}

// connect [<host:port>]
func handleConnect(ctx *ScriptContext, args []string) error {
	cfg := ctx.Store.Config()
	host, port, err := parseStartSession(args, &cfg)
	if err != nil {
		return fmt.Errorf("connect parse failed: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	if err = ctx.Session().Connect(addr); err != nil {
		return fmt.Errorf("connect exec failed: %w", err)
	}

	return nil
}

// listen [<host:port>]
func handleListen(ctx *ScriptContext, args []string) error {
	cfg := ctx.Store.Config()
	host, port, err := parseStartSession(args, &cfg)
	if err != nil {
		return fmt.Errorf("listen parse failed: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	if err = ctx.Session().Listen(addr); err != nil {
		return fmt.Errorf("listen exec failed: %w", err)
	}

	return nil
}

func handleDisconnect(ctx *ScriptContext, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("Syntax error, expected: `disconnect`")
	}

	ctx.Session().Close()
	return nil
}

func handleReset(ctx *ScriptContext, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("Syntax error, expected: `reset`")
	}

	if ctx.Reset == nil {
		return fmt.Errorf("Reset not implemented")
	}

	return ctx.Reset()
}

func handleSend(ctx *ScriptContext, args []string) error {
	msgStr, isRaw, err := parseSend(args)
	if err != nil {
		return fmt.Errorf("send parse failed: %w", err)
	}

	if msgStr == "" {
		return fmt.Errorf("invalid fix string, got empty")
	}

	delim := msgStr[len(msgStr)-1:]
	msg, err := message.MessageFromString(msgStr, delim)
	if err != nil {
		return fmt.Errorf("invalid fix string: %w", err)
	}

	return ctx.Session().Send(msg, isRaw)
}

func handleResetSequence(ctx *ScriptContext, args []string) error {
	seqNo, isInSeqNumUpdate, err := parseResetSequence(args)
	if err != nil {
		return fmt.Errorf("seq parse failed: %w", err)
	}

	// Get the latest snapshot for a partial SeqNo update
	sess := ctx.Session()
	snapshot := sess.Status()

	inSeq := snapshot.InSeqNum
	outSeq := snapshot.OutSeqNum

	if isInSeqNumUpdate {
		inSeq = seqNo
	} else {
		outSeq = seqNo
	}

	// Engine handles the state changes and emit the appropriate logs
	return sess.ResetSequence(inSeq, outSeq)
}

func init() {
	RegisterCommand("connect", handleConnect)       // connect [<host:port>]
	RegisterCommand("listen", handleListen)         // listen [<host:port>]
	RegisterCommand("disconnect", handleDisconnect) // disconnect
	RegisterCommand("reset", handleReset)           // reset
	RegisterCommand("seq", handleResetSequence)     // seq <in|out> <SeqNum>
	RegisterCommand("send", handleSend)             // send [-r] <msg>
}
