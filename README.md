# MicroFIX (MXShell)

A lightweight, developer-focused FIX (Financial Information eXchange) toolkit and CLI client. MXShell provides a powerful interactive REPL and a headless scripting engine for inspecting, generating, validating and automating FIX messages.

## ✨ Features

* 💻 **Interactive REPL:** Built-in shell with terminal history, arrow-key navigation and smart auto-completion.
* 📜 **Automated Scripting:** Headless execution of `.mxs` batch scripts with `expect`/`wait` semantics for automated FIX endpoint testing.
* 🧠 **Session Engine:** Fully-managed FIX state machine (Initiator & Acceptor), sequence tracking, and heartbeat management.
* 🔎 **Advanced Dictionary:** Query FIX specifications, search by regex, and generate valid sample messages instantly.
* ✅ **Validation & Decoding:** Decode raw FIX strings into readable trees and strictly validate them against FIX XML dictionaries.
* ⚡ **Variables & Aliases:** Reusable message templates, dynamic variables (`$UNIQUE`, `$TIMESTAMP`), and cross-message state extraction (`$LASTIN`).
* 📡 **Live Log Streaming:** View, tail, search, and save raw protocol logs in real-time.
* 🪶 **Tiny Footprint:** Easily compiled with TinyGo for a sub-megabyte, zero-dependency binary.
* 🧪 Deterministic behavior with extensive unit-tested session state transitions.

## Why MicroFIX?

MicroFIX is designed for developers who need:

* rapid FIX debugging
* deterministic scripting
* lightweight deployment
* strong protocol visibility
* programmable testing workflows

Unlike heavyweight GUI-first FIX tools, MicroFIX focuses on automation, observability and developer ergonomics. It is a standalone, dependency-free binary that can be easily deployed and used on servers or integrated into automation workflows without complex coding or scripting expertise.

---

## 🚀 Getting Started

### Installation / Build from Source

You can build Microfix using standard Go, or compile it with TinyGo for an ultra-lightweight binary:

```bash
# Standard Go Build
go build -ldflags "-s -w" -o mxshell ./cmd/mxshell

# Ultra-lightweight TinyGo Build
tinygo build -opt=z -no-debug -o mxshell ./cmd/mxshell

# Compression for minimizing binary size
upx --best --lzma mxshell

```

### Usage Modes

**1. Interactive Shell (REPL)**
Start the interactive CLI to connect, send messages, and query the dictionary manually:

```bash
./mxshell

```

**2. Headless Scripting**
Run automated test scripts without entering the interactive UI:

```bash
./mxshell -f tests/logon_flow.mxs

```

---

## 🔄 Example Interactive Workflow

```text
$ mxshell

MFix> connect localhost:3000

─── Connect ─────────────────────────────────────
  Status : OK
  Remote : localhost:3000
──────────────────────────────────────────────────

MFix> status

─── Session Status ──────────────────────────────
  State      : Active
  Sequence   : In(2) | Out(2)
  Activity   : Last In: 2s ago | Last Out: 2s ago
──────────────────────────────────────────────────

MFix> logs stream

─── Log Stream (Ctrl+C to exit) ────────────────
[2026-05-22 07:57:00.697] RECV << [Heartbeat] 8=FIX.4.4|9=51|35=0|49=SERVER|56=CLIENT|34=2|52=20260522-02:27:00|10=153|
[2026-05-22 07:57:17.157] SEND >> [Heartbeat] 8=FIX.4.4|9=55|35=0|49=CLIENT|56=SERVER|34=2|52=20260522-02:27:17.157|10=112|
[2026-05-22 07:57:30.010] RECV << [Quote] 8=FIX.4.4|9=60|35=S|49=SERVER|56=CLIENT|34=3|52=20260522-02:27:30|117=|55=|10=063|
[2026-05-22 07:58:00.122] RECV << [TestRequest] 8=FIX.4.4|9=63|35=1|49=SERVER|56=CLIENT|34=5|52=20260522-02:28:00|112=TESTING|10=145|
[2026-05-22 07:58:00.122] SEND >> [Heartbeat] 8=FIX.4.4|9=67|35=0|49=CLIENT|56=SERVER|34=4|52=20260522-02:28:00.122|112=TESTING|10=086|
[2026-05-22 07:58:16.374] RECV << [Logout] 8=FIX.4.4|9=51|35=5|49=SERVER|56=CLIENT|34=7|52=20260522-02:28:16|10=171|

MFix> status

─── Session Status ──────────────────────────────
  State      : Closed
  Sequence   : In(8) | Out(6)
  Activity   : Last In: 9s ago | Last Out: 9s ago
──────────────────────────────────────────────────

MFix> exit

```

---

## 💻 Interactive Commands (REPL)

Once inside the interactive `mxshell`, you can type `help` or `help <command>` for detailed information and utilize smart auto-completion.

### 🔌 Session Management

* `connect [<host:port>]` - Initiate a TCP connection to the target.
* `listen [<host:port>]` - Listen on a local port for an incoming connection.
* `disconnect` - Close the active network connection.
* `reset` - Close the current session and initialize a new one.
* `status` - Display current session state, uptime, and sequence numbers.
* `seq [<in|out> <SeqNum>]` - View or manually override FIX sequence numbers.

### 🔍 FIX Dictionary Tools

* `fix search <regex>` - Search the loaded FIX dictionary for tags or message names.
* `fix meta <header|trailer>` - View standard header/trailer structures.
* `fix decode <msg>` - Decode a raw FIX string into a readable tree.
* `fix validate <msg>` - Validate a raw message against the spec.
* `fix finalize <msg>` - Automatically compute/fix `BodyLength` (9) and `CheckSum` (10).
* `fix <field|message|sample> <id>` - Query field info, message structure, or generate sample data.

### ⚙️ Messaging & Utilities

* `send [-r] [-a] <msg>` - Send a FIX message. (`-r` for raw bypass, `-a` for alias).
* `alias [list | add <name> <msg> | del <name>]` - View or update FIX message shortcuts.
* `config [load <path> | save <path> | set <key> <val>]` - Manage session configurations.
* `logs [stream | search <regex> | save <path> | clear | head <n> | tail <n>]` - Interact with session logs.
* `run [-q] <filepath>` - Execute an external script file from within the REPL.
* `clear` - Clear the terminal screen.

---

## 📜 Scripting Reference (.mxs)

Microfix includes a headless scripting engine designed for testing deterministic FIX flows. Scripts are executed line-by-line and can utilize standard session commands (`connect`, `send`, `reset`) alongside dedicated flow-control assertions.

**Example Script (`test.mxs`):**

```bash
# Connect and wait for the engine to handle logon
connect 127.0.0.1:4000
waitstatus Active

# Send a New Order Single using dynamic variables
send -r 35=D|11=$UNIQUE|34=$SEQ_OUT|55=AAPL|54=1|38=100|40=1|59=0|

# Wait for the Execution Report
wait 35=8

# Extract the OrderID from the incoming execution report and save it
set VARS.OrderID $LASTIN[8,37]
print Successfully received OrderID: $VARS.OrderID

```

### Script-Only Commands

These commands are specific to automation flows and control script execution:

* `expect <MsgLike>` - Fail if the *next* application message doesn't match. (Automatically ignores background Heartbeats & Test Requests).
* `wait <MsgLike>` - Block execution until a matching message is received.
* `waitstatus <state>` - Block until session enters state (`New`, `Listening`, `LoggingIn`, `Active`, `Stale`, `OutOfSync`, `Closed`).
* `assert <exp1> <exp2>` - Fail the script if the two expressions do not match.
* `sleep <millis>` - Pause execution for a specified duration.
* `set <key> <val>` - Set a variable in the local store (e.g., `set VARS.Symbol AAPL`).
* `print <val> [...]` - Print text or resolved variables to the console.
* `include <filepath>` - Include and execute another script file inline.

### Variables & Substitution

Variables can be injected into any command using the `$` prefix:

* **Store Vars:** `$VARS.<name>`, `$CFG.<name>`, `$ALIAS.<name>`, `$ENV.<name>`
* **Generators:** `$UNIQUE` (UUID), `$TIMESTAMP` (UTC), `$DATE` (YYYYMMDD), `$DATE[+N]` (Offset dates)
* **Session State:** `$SEQ_IN`, `$SEQ_OUT`, `$STATUS`
* **Cross-Message Extraction:** `$LASTIN[MsgType, Tag]`, `$LASTOUT[MsgType, Tag]` (e.g., `$LASTIN[8,39]` gets the `OrdStatus` from the last Execution Report).

---

## 🧩 Decoding Output Example

```
MFix> fix decode 8=FIX.4.4|9=120|35=V|49=SENDER|56=TARGET|34=1|52=20260522-12:00:00|146=2|55=AAPL|55=GOOG|10=000|

[HEADER]
   8    = FIX.4.4                  BeginString (STRING)
   9    = 120                      BodyLength (LENGTH)
   35   = V                        MsgType (STRING)        → MARKET_DATA_REQUEST
   ...

[BODY]
   146  = 2                        NoRelatedSym (NUMINGROUP)
     └── Group 1
       55   = AAPL                 Symbol (STRING)
     └── Group 2
       55   = GOOG                 Symbol (STRING)

[TRAILER]
   10   = 000                      CheckSum (STRING)

```

---

## 🏗 Architecture

* **`pkg/spec`** → FIX XML dictionary parsing, routing, and message validation.
* **`pkg/message`** → Zero-allocation focused raw FIX message representation and manipulation.
* **`pkg/session`** → Deterministic FIX session engine (State machine, Resend Requests, Sequence management).
* **`pkg/executor`** → AST-based Script execution, evaluation, and dynamic variable substitution.
* **`cmd/mxshell`** → Interactive CLI, Readline REPL, and command handlers.

---

## 🧪 Test Coverage

MicroFIX includes unit tests for all core logic, engine states, and parsing packages. While coverage is currently incomplete for the CLI command handlers, the underlying session and execution mechanisms are heavily tested.

You can check current code coverage by running:

```bash
$ go test ./... -coverprofile=coverage.out
    [github.com/infinage/microfix/cmd/mxshell](https://github.com/infinage/microfix/cmd/mxshell)        coverage: 0.0% of statements
    [github.com/infinage/microfix/cmd/mxshell/internal/handlers](https://github.com/infinage/microfix/cmd/mxshell/internal/handlers)        coverage: 0.0% of statements
ok      [github.com/infinage/microfix/pkg/ast](https://github.com/infinage/microfix/pkg/ast)    0.006s    coverage: 95.2% of statements
ok      [github.com/infinage/microfix/pkg/executor](https://github.com/infinage/microfix/pkg/executor)    0.913s    coverage: 71.3% of statements
    [github.com/infinage/microfix/pkg/executor/handlers](https://github.com/infinage/microfix/pkg/executor/handlers)        coverage: 0.0% of statements
ok      [github.com/infinage/microfix/pkg/message](https://github.com/infinage/microfix/pkg/message)    0.011s    coverage: 85.8% of statements
ok      [github.com/infinage/microfix/pkg/pretty](https://github.com/infinage/microfix/pkg/pretty)    0.266s    coverage: 72.4% of statements
ok      [github.com/infinage/microfix/pkg/ringbuf](https://github.com/infinage/microfix/pkg/ringbuf)    0.011s    coverage: 100.0% of statements
ok      [github.com/infinage/microfix/pkg/session](https://github.com/infinage/microfix/pkg/session)    0.764s    coverage: 76.2% of statements
ok      [github.com/infinage/microfix/pkg/spec](https://github.com/infinage/microfix/pkg/spec)    0.701s    coverage: 84.1% of statements
ok      [github.com/infinage/microfix/pkg/store](https://github.com/infinage/microfix/pkg/store)    0.016s    coverage: 81.0% of statements
ok      [github.com/infinage/microfix/pkg/transport](https://github.com/infinage/microfix/pkg/transport)    0.022s    coverage: 60.2% of statements

```

---

## 🛣️ Roadmap

* [ ] Next-Gen GUI transition using **Wails + Templ + HTMX**

---

## 📚 References

* [FIX Protocol Developer Specifications](https://fix.dev)
* [Minifix (Inspiration)](https://elato.se/minifix/doc.html)

---

## ⚖️ License

MIT License
