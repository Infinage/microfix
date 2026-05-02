package handlers

import (
	"fmt"
	"os"
	"time"

	"github.com/infinage/microfix/internal/mxshell/config"
	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/ringbuf"
	"github.com/infinage/microfix/pkg/session"
)

// Read from sesson and write into circular buffer
func startLogger(sess *session.Session, cb *ringbuf.CircularBuffer) {
	// Session closes logs on run loop exit
	for log := range sess.Log() {
		hint := ""
		if msg := log.Msg; msg != nil {
			msgType, _ := msg.Get(35)
			entry, ok := sess.Spec().Messages[msgType]
			if ok {
				hint = entry.Name
			}
		}
		cb.Write(log.String(hint))
	}
}

func NewSession(cfg *config.Config) (*session.Session, error) {
	// Create new session
	return session.NewSession(
		cfg.SpecPath,
		cfg.SenderCompID,
		cfg.TargetCompID,
		cfg.HeartbeatInt,
	)
}

func handleStatus(ctx *AppContext, _ []string) {
	snapshot := ctx.Session.Status()

	var stateColor string
	switch snapshot.State {
	case session.SessionActive:
		stateColor = "\033[32m" // Green
	case session.SessionLoggingIn, session.SessionStale:
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

	fmt.Println("\n─── Session Status ─────────────────────────────────")
	fmt.Printf("  State      : %s%s\033[0m\n", stateColor, snapshot.State)
	fmt.Printf("  Sequence   : In(%d) | Out(%d)\n", snapshot.InSeqNum, snapshot.OutSeqNum)
	fmt.Printf("  Activity   : Last In: %s | Last Out: %s\n", lastIn, lastOut)
	fmt.Println("────────────────────────────────────────────────────")
}

func handleSend(ctx *AppContext, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: send [-r] <FixString>")
		return
	}

	// Send raw fix string as is if '-r' is set
	rawMsg := args[len(args)-1]
	isRaw := false
	for _, a := range args {
		if a == "-r" {
			isRaw = true
		}
	}

	// Delimiter is typically the last character (e.g., |, ^, or \x01)
	delim := rawMsg[len(rawMsg)-1:]
	msg, err := message.MessageFromString(rawMsg, delim)
	if err != nil {
		fmt.Printf("Invalid FIX string: %v\n", err)
		return
	}

	ctx.Session.Send(msg, isRaw)
}

func handleConnect(ctx *AppContext, _ []string) {
	addr := fmt.Sprintf("%s:%d", ctx.Config.IpAddr, ctx.Config.Port)
	fmt.Printf("Connecting to %s...\n", addr)
	if err := ctx.Session.Connect(addr); err != nil {
		fmt.Printf("Connection failed: %v\n", err)
	} else {
		fmt.Println("TCP Connection established.")
		go startLogger(ctx.Session, ctx.Logs)
	}
}

func handleListen(ctx *AppContext, _ []string) {
	addr := fmt.Sprintf("%s:%d", ctx.Config.IpAddr, ctx.Config.Port)
	fmt.Printf("Listening on %s...\n", addr)
	if err := ctx.Session.Listen(addr); err != nil {
		fmt.Printf("Listen failed: %v\n", err)
	} else {
		fmt.Println("Client connected.")
		go startLogger(ctx.Session, ctx.Logs)
	}
}

func handleReset(ctx *AppContext, _ []string) {
	ctx.Session.Close() // close old session

	// Create new session
	s, err := NewSession(ctx.Config)
	if err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}

	ctx.Session = s
	fmt.Println("New session created")
}

func init() {
	RegisterCommandHandler("status", handleStatus)
	RegisterCommandHandler("send", handleSend)
	RegisterCommandHandler("connect", handleConnect)
	RegisterCommandHandler("listen", handleListen)
	RegisterCommandHandler("reset", handleReset)
}
