package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
)

func startLogger(sess *session.Session, cb *CircularBuffer) {
	for {
		select {
		case log, ok := <-sess.Log():
			if !ok {
				return
			}
			cb.Write(log.String())

		case <-sess.Done():
			// The session is done, but there might be logs left in the channel.
			// We need to "drain" them so we don't miss the "Ended" event.
			for {
				select {
				case log, ok := <-sess.Log():
					if !ok {
						return
					}
					cb.Write(log.String())
				default:
					return // No more logs currently in the buffer
				}
			}
		}
	}
}

func handleLogStream(cb *CircularBuffer) {
	fmt.Println("\n--- Log Stream (Ctrl+C to exit) ---")
	lastPtr := cb.ptr
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Println("\n--- Exiting Stream ---")
			return
		case <-ticker.C:
			cb.mu.Lock()
			currentPtr := cb.ptr
			for lastPtr != currentPtr {
				if cb.lines[lastPtr] != "" {
					fmt.Println(cb.lines[lastPtr])
				}
				lastPtr = (lastPtr + 1) % cb.size
			}
			cb.mu.Unlock()
		}
	}
}

func handleFixSearch(s *session.Session, pattern string) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		fmt.Printf("Invalid regex: %v\n", err)
		return
	}

	fmt.Printf("\n--- Spec Search: '%s' ---\n", pattern)

	// Search in Fields
	fmt.Println("\033[1m[ FIELDS ]\033[0m")
	fCount := 0
	for tag, field := range s.Spec().Fields {
		if re.MatchString(field.Name) || re.MatchString(strconv.Itoa(int(tag))) {
			fmt.Printf("  %-5d | %s\n", tag, field.Name)
			fCount++
		}
	}

	// Search in Messages
	fmt.Println("\n\033[1m[ MESSAGES ]\033[0m")
	mCount := 0
	for msgType, msgDef := range s.Spec().Messages {
		if re.MatchString(msgDef.Name) || re.MatchString(msgType) {
			fmt.Printf("  %-5s | %s\n", msgType, msgDef.Name)
			mCount++
		}
	}
	fmt.Printf("\nFound %d fields, %d messages.\n", fCount, mCount)
}

func handleLogSearch(cb *CircularBuffer, pattern string) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		fmt.Printf("Invalid regex: %v\n", err)
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	fmt.Printf("\n--- Log Search: '%s' ---\n", pattern)
	found := 0
	for i := 0; i < cb.size; i++ {
		idx := (cb.ptr + i) % cb.size
		line := cb.lines[idx]
		if line != "" && re.MatchString(line) {
			fmt.Println(line)
			found++
		}
	}
	if found == 0 {
		fmt.Println("No matches in buffer.")
	} else {
		fmt.Printf("\nTotal matches: %d\n", found)
	}
}

func handleStatus(sess *session.Session, cfg *Config) {
	var stateColor string
	switch sess.Status() {
	case session.SessionActive:
		stateColor = "\033[32m" // Green
	case session.SessionLoggingIn, session.SessionStale:
		stateColor = "\033[33m" // Yellow
	default:
		stateColor = "\033[31m" // Red
	}

	fmt.Println("\n─── Session Status ─────────────────────────────────")
	fmt.Printf("  Target ID : \033[1m%s\033[0m\n", cfg.TargetCompID)
	fmt.Printf("  State     : %s%s\033[0m\n", stateColor, sess.Status().String())
	fmt.Printf("  Heartbeat : %d seconds\n", cfg.HeartbeatInt)
	fmt.Println("────────────────────────────────────────────────────")
}

func handleSend(s *session.Session, args []string) {
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

	// Delimiter is typically the last character (e.g., |, ^, or \x01)
	delim := rawMsg[len(rawMsg)-1:]
	msg, err := message.MessageFromString(rawMsg, delim)
	if err != nil {
		fmt.Printf("Invalid FIX string: %v\n", err)
		return
	}

	s.Send(msg, isRaw)
}

func handleSpecHelp(s *session.Session, cfg Config, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: fix field <tag> | fix message <MsgType> | fix sample <MsgType>")
		return
	}

	sub := strings.ToLower(args[0])
	id := args[1]

	switch sub {
	case "field":
		tag, _ := strconv.Atoi(id)
		if f, ok := s.Spec().Fields[uint16(tag)]; ok {
			WritePrettyFieldDef(os.Stdout, f)
		} else {
			fmt.Printf("Field %s not found\n", id)
		}
	case "message":
		if m, ok := s.Spec().Messages[id]; ok {
			WritePrettySpecEntry(os.Stdout, m, s.Spec().FieldNames, cfg.SpecDisplayOptFields, 0)
		} else {
			fmt.Printf("Message %s not found\n", id)
		}
	case "sample":
		if smp, err := s.Spec().Sample(id, spec.SampleOptions{IncludeOptional: true}); err == nil {
			fmt.Println(smp.String("|"))
		} else {
			fmt.Println("Sample failed:", err)
		}
	default:
		fmt.Println("2nd argument must be one of field, message, sample")
	}
}

// --- Main Application ---

func main() {
	fmt.Println(`
 __       __  __    __         ______   __                   __  __ 
|  \     /  \|  \  |  \       /      \ |  \                 |  \|  \
| $$\   /  $$| $$  | $$      |  $$$$$$\| $$____   ______  | $$| $$
| $$$\ /  $$$ \$$\/  $$______| $$___\$$| $$    \ /      \ | $$| $$
| $$$$\  $$$$  >$$  $$|      \\$$    \ | $$$$$$$\|  $$$$$$\| $$| $$
| $$\$$ $$ $$ /  $$$$\ \$$$$$$_\$$$$$$\| $$  | $$| $$    $$| $$| $$
| $$ \$$$| $$|  $$ \$$\      |  \__| $$| $$  | $$| $$$$$$$$| $$| $$
| $$  \$ | $$| $$  | $$       \$$    $$| $$  | $$ \$$     \| $$| $$
 \$$      \$$ \$$   \$$        \$$$$$$  \$$   \$$  \$$$$$$$ \$$ \$$
	`)

	cfg := InitConfig()
	cb := NewCircularBuffer(1000)

	sess, err := session.NewSession(cfg.SpecPath, cfg.SenderCompID, cfg.TargetCompID, cfg.HeartbeatInt)
	if err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("MFix> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		args := strings.Fields(input)
		cmd := strings.ToLower(args[0])

		switch cmd {
		case "exit", "quit":
			sess.Close()
			return

		case "connect":
			addr := fmt.Sprintf("%s:%d", cfg.IpAddr, cfg.Port)
			fmt.Printf("Connecting to %s...\n", addr)
			if err := sess.Connect(addr); err != nil {
				fmt.Printf("Connection failed: %v\n", err)
			} else {
				fmt.Println("TCP Connection established.")
				go startLogger(sess, cb)
			}

		case "listen":
			addr := fmt.Sprintf("%s:%d", cfg.IpAddr, cfg.Port)
			fmt.Printf("Listening on %s...\n", addr)
			if err := sess.Listen(addr); err != nil {
				fmt.Printf("Listen failed: %v\n", err)
			} else {
				fmt.Println("Client connected.")
				go startLogger(sess, cb)
			}

		case "disconnect":
			sess.Close()

		case "reset":
			sess.Close() // close old session

			// Create new session
			s, err := session.NewSession(cfg.SpecPath, cfg.SenderCompID, cfg.TargetCompID, cfg.HeartbeatInt)
			if err != nil {
				fmt.Printf("Critical Error: %v\n", err)
				os.Exit(1)
			}

			sess = s
			fmt.Println("New session created")

		case "send":
			handleSend(sess, args[1:])

		case "status":
			handleStatus(sess, &cfg)

		case "logs":
			if len(args) < 2 {
				fmt.Println("\n--- Session Logs ---")
				cb.Dump(os.Stdout)
				continue
			}

			sub := strings.ToLower(args[1])
			switch sub {
			case "-f":
				handleLogStream(cb)
			case "search":
				if len(args) < 3 {
					fmt.Println("Usage: logs search <regex>")
				} else {
					handleLogSearch(cb, args[2])
				}
			default:
				fmt.Printf("Unknown logs subcommand: %s\n", sub)
			}

		case "fix":
			if len(args) < 2 {
				fmt.Println("Usage: fix [field|message|sample|search] <id/pattern>")
				continue
			}

			sub := strings.ToLower(args[1])
			// New 'search' case added here
			if sub == "search" {
				if len(args) < 3 {
					fmt.Println("Usage: fix search <regex>")
				} else {
					handleFixSearch(sess, args[2])
				}
				continue
			}

			// Existing help logic (field, message, sample)
			if len(args) < 3 {
				fmt.Println("Usage: fix [field|message|sample] <id>")
			} else {
				handleSpecHelp(sess, cfg, args[1:])
			}

		default:
			fmt.Printf("Unknown command: %s\n", cmd)
		}
		fmt.Println()
	}
}
