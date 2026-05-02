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

	fmt.Println("\n─── Session Status ──────────────────────────────")
	fmt.Printf("  State      : %s%s\033[0m\n", stateColor, snapshot.State)
	fmt.Printf("  Sequence   : In(%d) | Out(%d)\n", snapshot.InSeqNum, snapshot.OutSeqNum)
	fmt.Printf("  Activity   : Last In: %s | Last Out: %s\n", lastIn, lastOut)
	fmt.Println("──────────────────────────────────────────────────")
}

func handleSend(ctx *AppContext, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: send [-r] <FixString>")
		return
	}

	rawMsg := args[len(args)-1]
	isRaw := false
	for _, a := range args {
		if a == "-r" {
			isRaw = true
		}
	}

	fmt.Println("\n─── Send Message ────────────────────────────────")

	delim := rawMsg[len(rawMsg)-1:]
	msg, err := message.MessageFromString(rawMsg, delim)
	if err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("──────────────────────────────────────────────────")
		return
	}

	ctx.Session.Send(msg, isRaw)

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

func handleConnect(ctx *AppContext, _ []string) {
	addr := fmt.Sprintf("%s:%d", ctx.Config.IpAddr, ctx.Config.Port)

	fmt.Println("\n─── Connect ─────────────────────────────────────")

	if err := ctx.Session.Connect(addr); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	} else {
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  Remote : %s\n", addr)
		go startLogger(ctx.Session, ctx.Logs)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func handleListen(ctx *AppContext, _ []string) {
	addr := fmt.Sprintf("%s:%d", ctx.Config.IpAddr, ctx.Config.Port)

	fmt.Println("\n─── Listen ──────────────────────────────────────")

	if err := ctx.Session.Listen(addr); err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
	} else {
		fmt.Printf("  Status : OK\n")
		fmt.Printf("  Bound  : %s\n", addr)
		fmt.Println("  Info   : Waiting for client connection...")
		go startLogger(ctx.Session, ctx.Logs)
	}

	fmt.Println("──────────────────────────────────────────────────")
}

func handleReset(ctx *AppContext, _ []string) {
	fmt.Println("\n─── Session Reset ───────────────────────────────")

	ctx.Session.Close()

	s, err := NewSession(ctx.Config)
	if err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("──────────────────────────────────────────────────")
		os.Exit(1)
	}

	ctx.Session = s

	fmt.Printf("  Status : OK\n")
	fmt.Println("  Info   : New session initialized")

	fmt.Println("──────────────────────────────────────────────────")
}

func init() {
	RegisterCommandHandler("status", handleStatus)
	RegisterCommandHandler("send", handleSend)
	RegisterCommandHandler("connect", handleConnect)
	RegisterCommandHandler("listen", handleListen)
	RegisterCommandHandler("reset", handleReset)
}
