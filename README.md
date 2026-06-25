<p align="center">
  <img src="cmd/mxgui/assets/image/logo.png" width="240" alt="MicroFIX Logo"/>
</p>

<h1 align="center">MicroFIX</h1>

<p align="center">
A high-performance FIX (Financial Information eXchange) workstation suite for developers, testers, and quants.
</p>

<p align="center">
Real-time GUI • Interactive Shell • Deterministic Automation • Cross Platform
</p>

<br/>

<div align="center">
  <h3><a href="https://github.com/user-attachments/assets/cc71c724-28a4-4fa6-9775-c6da18e87696">MXGUI: Graphical User Interface</a></h3>
  <video src="https://github.com/user-attachments/assets/cc71c724-28a4-4fa6-9775-c6da18e87696" controls muted autoplay playsinline width="800"></video>
</div>

<br/>

---

## Overview

MicroFIX combines a modern desktop workstation with a powerful command-line shell, both powered by the same deterministic FIX engine.

* **`mxgui`** — Real-time monitoring, message inspection, validation, and script execution.
* **`mxshell`** — Interactive REPL and headless automation for testing and CI pipelines.
* **Shared Core Engine** — Session management, validation, scripting, aliases, and protocol handling.

Unlike heavyweight legacy FIX tools, MicroFIX ships as lightweight native binaries with no Java or Electron runtime dependencies.

---

# ✨ Features

## 🖥 Desktop GUI (`mxgui`)

### Real-Time Monitoring

* Zero-lag log streaming via Server-Sent Events.
* Handles thousands of messages smoothly.
* Stream filtering and export capabilities.

### Deep Message Inspection

* Visual tree view of repeating groups and component blocks.
* Decoded tag names and field metadata.
* Syntax-highlighted raw FIX view.
* Side-by-side message diff inspector.
* Message validator and finalizer tools.
* Dictionary browser and search.

### Offline Utilities

Many tools work without a live session:

* FIX decode
* Validation
* BodyLength and CheckSum finalization
* Dictionary browsing
* Message comparison
* Script execution

### Script Runner

The same engine powering `mxshell` is available directly from the GUI.

* Execute `.mxs` scripts
* Observe execution visually
* Reuse existing automation flows

### Native Experience

Built with Wails + HTMX + Tailwind.

* Lightweight memory footprint
* Fast startup
* Modern aesthetics
* Native Windows and Linux builds

---

## 💻 Interactive Shell (`mxshell`)

<div align="center">
  <h3><a href="https://github.com/user-attachments/assets/74aa908e-ef16-4a6f-8be0-2ee49c3f57cd">MXShell: Command Line Interface</a></h3>
  <video src="https://github.com/user-attachments/assets/74aa908e-ef16-4a6f-8be0-2ee49c3f57cd" controls muted autoplay playsinline width="800"></video>
</div>

### Smart REPL

* Command history
* Arrow-key navigation
* Smart auto-completion
* Help system

### Log Management

* Live log streaming
* Regex searches
* Head and tail views
* Export to files

### Scripting

* Deterministic execution
* Script inclusion (`include`)
* Assertions and waiting primitives
* Batch execution for CI/CD

### Variables

Dynamic substitutions:

* `$UNIQUE`
* `$TIMESTAMP`
* `$DATE`
* `$SEQ_IN`
* `$SEQ_OUT`
* `$STATUS`
* `$LASTIN[...]`
* `$LASTOUT[...]`

### Aliases

Create reusable message templates and shortcuts.

---

## 🧠 Shared Core Engine

### Session Management

* Initiator and Acceptor modes
* Heartbeat management
* Sequence tracking
* Resend handling
* State machine driven architecture

### Automatic Administrative Messages

MicroFIX automatically handles:

* Logon
* Logout
* Heartbeats
* Test Requests
* Sequence Reset
* Resend Requests

allowing scripts and users to focus primarily on application-level messages.

### Flexible Message Input

Paste messages using:

```text
35=D|55=AAPL|54=1|38=100|
```

and MicroFIX automatically:

* Converts delimiters to SOH
* Computes BodyLength
* Computes CheckSum
* Normalizes formatting

### Validation Modes

Three validation levels are available:

| Level  | Description                      |
| ------ | -------------------------------- |
| None   | No validation                    |
| Basic  | Structural checks                |
| Strict | Full dictionary-based validation |

### Dictionary Support

* Tag lookup
* Message metadata
* Regex search
* Decode raw FIX strings
* Generate sample messages

---

# Supported FIX Versions

| Version     |
| ----------- |
| FIX 4.0     |
| FIX 4.1     |
| FIX 4.2     |
| FIX 4.3     |
| FIX 4.4     |
| FIX 5.0     |
| FIX 5.0 SP1 |
| FIX 5.0 SP2 |

## Custom Dictionaries

MicroFIX is dictionary-driven and supports custom XML specifications.

This enables:

* Venue-specific extensions
* Proprietary message types
* Internal FIX dialects
* Custom fields and components

Any protocol following the standard FIX XML schema can be loaded.

---

# Why MicroFIX?

### Inspect Faster

Deeply inspect messages, repeating groups, and field metadata with a modern GUI.

### Automate Reliably

Deterministic scripts with `wait`, `expect`, assertions, variables, and includes.

### Work Anywhere

Run the GUI on your desktop or the shell inside CI pipelines.

### One Engine Everywhere

GUI and CLI share the exact same protocol engine and scripting runtime.

---

# 🚀 Getting Started

## Requirements

* Go 1.26+
* CGO enabled for `mxgui`
* WebKitGTK 6.0 development libraries on Linux

Example:

```bash
sudo apt install libwebkitgtk-6.0-dev
```

## Build

### Desktop GUI

```bash
go build -tags desktop,production -ldflags="-s -w" -o mxgui ./cmd/mxgui
```

### CLI

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o mxshell ./cmd/mxshell
```

### Optional Compression

```bash
upx --best mxgui mxshell
```

---

# Scripting

MicroFIX includes a deterministic scripting language for automated FIX flows.

```bash
connect 127.0.0.1:4000

waitstatus Active

send -r 35=D|11=$UNIQUE|34=$SEQ_OUT|55=AAPL|54=1|38=100|40=1|

wait 35=8

set VARS.OrderID $LASTIN[8,37]

print Received OrderID: $VARS.OrderID
```

### Script Commands

* `expect`
* `wait`
* `waitstatus`
* `assert`
* `sleep`
* `set`
* `print`
* `include`

---

# Architecture

```text
cmd/mxgui
    Desktop GUI and script runner

cmd/mxshell
    Interactive REPL and automation

pkg/spec
    Dictionary parsing and validation

pkg/message
    Zero-allocation FIX message representation

pkg/session
    Session engine and state machine

pkg/executor
    Script parser and evaluator
```

---

# Roadmap

* [x] Wails + HTMX desktop workstation
* [ ] Improve documentation
* [ ] Improve test coverage
* [ ] Additional script assertions
* [ ] GUI performance improvements

---

# License

MIT License
