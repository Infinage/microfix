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
		case msg, ok := <-sess.Incoming():
			if !ok {
				return
			}
			cb.Write("RECV << " + msg.String("|"))
		case err, ok := <-sess.Errors():
			if !ok {
				return
			}
			cb.Write("ERR  !! " + err.Error())
		case <-sess.Done():
			cb.Write("SYS  .. Session Closed")
			return
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

func handleSearch(s *spec.Spec, pattern string) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		fmt.Printf("Invalid regex: %v\n", err)
		return
	}

	found := false
	for tag, field := range s.Fields {
		tagStr := strconv.Itoa(int(tag))
		if re.MatchString(field.Name) || re.MatchString(tagStr) {
			fmt.Printf("  [%-5d] %-20s (%s)\n", tag, field.Name, field.Type)
			found = true
		}
	}
	if !found {
		fmt.Println("No matches found.")
	}
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

func handleSpecHelp(s *spec.Spec, cfg Config, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: fix field <tag> | fix message <MsgType> | fix sample <MsgType>")
		return
	}

	sub := strings.ToLower(args[0])
	id := args[1]

	switch sub {
	case "field":
		tag, _ := strconv.Atoi(id)
		if f, ok := s.Fields[uint16(tag)]; ok {
			WritePrettyFieldDef(os.Stdout, f)
		} else {
			fmt.Printf("Field %s not found\n", id)
		}
	case "message":
		if m, ok := s.Messages[id]; ok {
			WritePrettySpecEntry(os.Stdout, m, s.FieldNames, cfg.SpecDisplayOptFields, 0)
		} else {
			fmt.Printf("Message %s not found\n", id)
		}
	case "sample":
		if smp, err := s.Sample(id, spec.SampleOptions{IncludeOptional: true}); err == nil {
			fmt.Println(smp.String("|"))
		} else {
			fmt.Println("Sample failed:", err)
		}
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

	s := sess.Spec()
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

		case "send":
			handleSend(sess, args[1:])

		case "status":
			fmt.Printf("Target: %s | Heartbeat: %ds\n", cfg.TargetCompID, cfg.HeartbeatInt)

		case "logs":
			if len(args) > 1 && args[1] == "-f" {
				handleLogStream(cb)
			} else {
				fmt.Println("\n--- Session Logs ---")
				cb.Dump(os.Stdout)
			}

		case "search":
			if len(args) < 2 {
				fmt.Println("Usage: search <regex>")
			} else {
				handleSearch(&s, args[1])
			}

		case "fix":
			handleSpecHelp(&s, cfg, args[1:])

		default:
			fmt.Printf("Unknown command: %s\n", cmd)
		}
		fmt.Println()
	}
}
