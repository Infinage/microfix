# MicroFIX

A lightweight, developer-focused FIX (Financial Information eXchange) toolkit with a powerful CLI for inspecting, generating, and validating FIX messages.

## ✨ Features

* 🔍 **Decode FIX messages** into a structured, human-readable format
* 📘 **Query FIX specifications** (fields, messages, header, trailer)
* 🧪 **Generate sample messages** from spec definitions
* ✅ **Validate FIX messages** (basic & strict modes)
* 🔎 **Search FIX dictionary** using regex
* ⚡ **Alias system** for reusable message templates
* 🧠 **Session engine** with sequence tracking and heartbeat handling
* 📜 **Persistent command history**
* 🪶 **Tiny binary (~300 KB)** built with TinyGo

---

## 🚀 Getting Started

### Build from Source

```bash
tinygo build -opt=z -no-debug -panic=trap -o mxshell github.com/infinage/microfix/cmd/mxshell
strip mxshell
upx --best --lzma mxshell
```

### Run

```bash
./mxshell
```

---

## 📦 Prebuilt Binaries

Prebuilt binaries will be available in the repository soon for:

* Linux (x86_64)
* Windows (x86_64)

---

## 💻 CLI Usage

### General

```
MFix> <command> [args]
```

---

### FIX Commands

```
fix search <regex>
fix meta [header|trailer]
fix decode <fixMessage>
fix validate <fixMessage>
fix [field|message|sample] <id>
```

#### Example

```bash
fix decode 8=FIX.4.4|9=120|35=V|49=SENDER|56=TARGET|34=1|52=...|10=000|
```

---

### Alias Commands

Create shortcuts for frequently used FIX messages.

```
alias list
alias add <name> <fixMessage>
alias delete <name> [name2 ...]
alias save [path]
```

---

### Config Commands

```
config
config load <path>
config save <path>
config set <key> <value>
```

---

## 🧩 Example Output

```
[HEADER]
   8    = FIX.4.4         BeginString (STRING)
   9    = 120             BodyLength (LENGTH)
   35   = V               MsgType (STRING)        → MARKET_DATA_REQUEST

[BODY]
   146  = 2               NoRelatedSym (NUMINGROUP)
     └── Group 1
       55   = AAPL        Symbol (STRING)
     └── Group 2
       55   = GOOG        Symbol (STRING)

[TRAILER]
   10   = 000             CheckSum (STRING)
```

---

## 🏗 Architecture

* **pkg/spec** → FIX spec parsing & validation
* **pkg/message** → FIX message representation
* **pkg/session** → FIX session engine (state machine)
* **pkg/pretty** → Structured CLI rendering
* **mxshell** → Interactive REPL interface

---

## ⚙️ Configuration

Config is managed via `.mxrc`:

* Validation mode (strict / basic)
* Sample generation options
* Display preferences

---

## Roadmap

* [ ] Fix protocol correctness
* [ ] Batch execution / scripting
* [ ] GUI (Svelte + WebView + C++)

---

## 📚 References

* [https://fix.dev](https://fix.dev)
* [https://elato.se/minifix/doc.html](https://elato.se/minifix/doc.html)

---

## ⚖️ License

MIT License
