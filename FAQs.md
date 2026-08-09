# Frequently Asked Questions

> Looking for the product overview? See the main [README](README.md).

## Table of Contents

- [Getting Started](#getting-started)
- [Configuration (`.mxrc`)](#configuration-mxrc)
- [Custom Dictionaries / Vendor Specs](#custom-dictionaries--vendor-specs)
- [Scripting & Macros](#scripting--macros)
- [GUI-Specific](#gui-specific)
- [FIX Protocol](#fix-protocol)
- [Troubleshooting](#troubleshooting)

---

## Getting Started

### What is MicroFIX and how does it differ from QuickFIX/other FIX engines?

Unlike QuickFIX(/J/n) where you have to write an entire application layer against an API just to get started, **MicroFIX is a complete, out-of-the-box workstation**. Connecting, sending, validating, and scripting are all available immediately with zero code required. It acts as a native desktop GUI (`mxgui`) and an interactive CLI (`mxshell`) built on top of the *same* deterministic session, scripting, and validation engine. 

### What's the difference between MXGUI and MXShell - which should I use?

They share the exact same engine, so behavior is identical either way.

- **MXGUI** - best for interactive/manual work: live log streaming, message inspection/diffing, dictionary browsing, and visually building/running scripts.
- **MXShell** - best for headless automation: CI pipelines, batch `.mxs` scripts, quick REPL-style testing over SSH.

### How do I install/build MicroFIX from source vs. pre-built binaries?

Pre-built binaries are on the [Releases](https://github.com/Infinage/microfix/releases) page. To build from source see the [Installation](README.md#installation) section - `go install` for either tool, or a manual `go build` (the GUI needs CGO + WebKitGTK on Linux/macOS via Wails v3; the CLI is a plain `CGO_ENABLED=0` build).

### What FIX versions are supported out of the box?

MicroFIX is fully dictionary-driven. It supports standard FIX versions natively and can load custom XML dictionaries for venue-specific extensions or proprietary dialects. Use the following exact values in your `.mxrc` file to load the internal dictionaries:

| Protocol | Spec Config Value |
| --- | --- |
| FIX 4.0 | `FIX40` |
| FIX 4.1 | `FIX41` |
| FIX 4.2 | `FIX42` |
| FIX 4.3 | `FIX43` |
| FIX 4.4 | `FIX44` |
| FIXT 1.1 | `FIXT11` |
| FIX 5.0 | `FIX50` |
| FIX 5.0 SP1 | `FIX50SP1` |
| FIX 5.0 SP2 | `FIX50SP2` |

---

## Configuration (`.mxrc`)

### Where does MicroFIX look for its config file, and what's the precedence?

Both tools check `./.mxrc` (current directory) and `~/.mxrc` (home directory). If neither exists, sensible defaults are used, so you can start MicroFIX with zero setup.

### What are the default configuration values?

| Setting | Default |
| --- | --- |
| SenderCompID | `SENDER` |
| TargetCompID | `TARGET` |
| FIX Version | `FIX44` |
| Heartbeat Interval | `30s` |
| Listen Address | `0.0.0.0:1234` |
| Script Timeout | `5s` |
| Validation Mode | `Strict` |

### What does a full `.mxrc` file look like, and which fields are required?

It's a flat JSON file. Common fields:

```jsonc
{
  "SenderCompID": "SENDER",
  "TargetCompID": "TARGET",
  "HeartbeatInt": 30,
  "SessionSpec": "FIX44",       // or a path to a custom XML dictionary
  "ApplicationSpec": "FIX44",
  "FixValidateStrict": true,
  "IpAddr": "0.0.0.0",
  "Port": 1234,
  "Alias": { "MyOrder": "35=D|55=AAPL|54=1|38=100|40=2|" }
}

```

You can edit this by hand, through the MXGUI Session Settings page, or via the MXShell `config` command.

### How do I switch between Initiator and Acceptor mode?

This is controlled by whether MicroFIX connects out to `IpAddr:Port` or listens on it - use the `connect` vs `listen` commands in MXShell (or the equivalent buttons in MXGUI's session controls).

### How do I point MicroFIX at a custom IP/port for a specific venue?

Set `IpAddr`/`Port` in `.mxrc`, or override them per-session via the CLI `config`/`connect` commands or the GUI Session Settings page.

---

## Custom Dictionaries / Vendor Specs

### How do I import my own custom vendor-specific ROE/XML dictionary?

Set `SessionSpec` and/or `ApplicationSpec` to an absolute or relative path to your XML file instead of one of the built-in names (e.g. `FIX44`). MicroFIX is fully dictionary-driven, so custom message types, components, and fields defined in your XML are immediately available for validation, the Dictionary Browser, and message sampling.

### What's the difference between `SessionSpec` and `ApplicationSpec`?

`SessionSpec` covers admin-level messages (Logon, Heartbeat, Sequence Reset, etc.), while `ApplicationSpec` covers business messages (orders, executions, etc.). Splitting the two is what allows a FIXT1.1 session layer to be paired with, say, a FIX50/FIX50SP1/FIX50SP2 application layer.

### Can I use a FIXT1.1 session layer with a custom FIX50-family application dictionary?

Yes - set `SessionSpec: "FIXT11"` and `ApplicationSpec` to whichever FIX50 variant (or custom XML path) your venue uses.

### How strict is dictionary validation, and how do I relax it for a non-conformant venue?

Validation always runs at least at a **Basic** level (checksum, body length, required fields, repeating groups). Setting `FixValidateStrict: true` (the default) additionally enforces **Strict** checks - field type checking and rejection of unknown fields. Set it to `false` if your venue's messages don't fully conform to the dictionary but you still want basic structural checks.

### Migrating from MiniFIX: how do I convert my existing MiniFIX XML export into a microfix `.mxrc`?

**Work in progress.** A converter is planned that will translate MiniFIX's `baseConf` (SenderCompID/TargetCompID/heartbeat/FIX version/connect history) and `transConf` templates directly into an equivalent `.mxrc`, including `Alias` entries. Until it ships, recreate your session config and templates by hand using the `.mxrc` format shown above.

---

## Scripting & Macros

### Where can I find the full list of global substitution variables?

Variables can be injected into scripts, CLI commands, or GUI inputs using the `$` prefix.

**System & State**

| Variable | Description |
| --- | --- |
| `$UNIQUE` | Generates a random UUID (e.g. for `ClOrdID`). |
| `$UNIQUE[N]` | Generates a random alphanumeric string of length `N` (maximum 1000). |
| `$TIMESTAMP` | Current UTC timestamp in `YYYYMMDD-HH:MM:SS.000` format. |
| `$DATE` | Current date in `YYYYMMDD` format. |
| `$DATE[+N]` | Current date offset by `N` days (e.g. `$DATE[+1]` is tomorrow). |
| `$STATUS` | Current session state (e.g. `Active`, `Closed`). |
| `$SEQ_IN` | Current internal inbound sequence number. |
| `$SEQ_OUT` | Current internal outbound sequence number. |
| `$ERROR` | Error message from the most recent failed condition. |

**Context & Store**

| Variable | Description |
| --- | --- |
| `$CFG.<key>` | Reads a value from the session configuration. |
| `$VARS.<key>` | Reads a script-defined variable created with the `set` command. |
| `$ALIAS.<name>` | Expands a saved alias. |
| `$ENV.<name>` | Reads an environment variable. |
| `$BUF` | The complete raw FIX message currently in the buffer. |

**Message Context (Tag Extraction & Slicing)**
`$BUF`, `$LASTIN`, and `$LASTOUT` support tag extraction and string slicing using bracket notation.

| Syntax | Description |
| --- | --- |
| `$BUF[Tag]` | Extracts the first occurrence of `Tag` from the buffered message. |
| `$BUF[Tag,Inst]` | Extracts the `Inst`-th occurrence of `Tag`. |
| `$BUF[Tag,Inst,End]` | Returns characters from index `0` through `End`. |
| `$BUF[Tag,Inst,Start,End]` | Returns characters from `Start` through `End`. |
| `$LASTIN[Msg,Tag,...]` | Extracts or slices a tag from the last incoming message of type `Msg`. |
| `$LASTOUT[Msg,Tag,...]` | Extracts or slices a tag from the last outgoing message of type `Msg`. |

*(Note: The instance number defaults to `1` when omitted. Tag instances are counted in message order, starting from `1`. Slicing uses zero-based string indexes.)*

### Are macro prefixes case-sensitive?

No - `$VARS`, `$vars`, and `$VaRs` are all treated the same, as are all other macro prefixes. Arguments, values, and payload contents (e.g. bracket contents like the message type in `$LASTIN[d,11]`) remain case-sensitive.

### How do aliases work, and how do I parameterize them?

Aliases are reusable FIX message templates stored under `$ALIAS.<name>` in `.mxrc` (or set at runtime). Rather than hardcoding values, combine them with macros so each send resolves fresh data instead of stale, copy-pasted fields.

```bash
# Define the alias
set $ALIAS.AAPL 35=D|55=AAPL|54=1|38=$UNIQUE[3]|40=2|11=$UNIQUE|

# Invoke it in MXShell
send -a AAPL

# Invoke in scripts
send $ALIAS.AAPL

```

A common pattern is a QuoteRequest/Quote flow where you want to immediately act on whatever quote you just received. Build the responding alias around `$LASTIN[...]` referencing the incoming Quote message, and MicroFIX resolves it against whatever was actually last received at send time.

Aliases go beyond flat substitution too:

* **Slicing** - pull a specific byte range out of a message with `$BUF[Tag,Instance,Start,End]`, useful when a template only needs part of an existing payload.
* **Randomized values** - `$UNIQUE[N]` generates a random value up to `N`, handy for order IDs, quantities, or client order refs.
* **Nested macros** - an alias's resolved value is itself re-scanned for further macros (bounded to a safe depth to guard against circular references), so aliases can reference other aliases or variables in combination.

### Can I slice values extracted from FIX messages?

Yes. For example, `$LASTIN[V,52,1,8]` extracts the first eight characters of tag `52`. This is useful when a FIX timestamp or other field contains multiple pieces of information and only part of the value is required.

### How do I reference the last received/sent message's fields in a script or alias?

Use `$LASTIN[MsgType,Tag,Instance]` / `$LASTOUT[MsgType,Tag,Instance]`, e.g. `$LASTIN[8,17]` grabs tag 17 (ExecID) from the last incoming Execution Report.

### How do I load a specific past message into the buffer, without waiting for a new one?

Use `loadmsg <in|out> <id>` to pull a specific message from session history straight into the buffer, so `$BUF[...]` can extract from it. This is useful when you want to re-inspect or re-slice an older message without needing to trigger a fresh `wait`/`expect` (which are the other two ways the buffer gets populated).

### How do I split a large script into multiple files?

Use `include <path>` to pull in and execute another script file inline - handy for sharing common setup (connecting, logon, common aliases) across several test scripts instead of duplicating it in each one.

### How do I invert the result of a command or condition?

Prefix it with `not`, e.g. `not isset VARS.Foo` or `if not assert 1 == 2`. It succeeds whenever the wrapped command would have failed, and vice versa.

### How do I manually override the session's sequence numbers mid-test?

Use `seq in <SeqNum>` / `seq out <SeqNum>` (or the equivalent `ResetSeqNumFlag`/reset options via `reset`). Moving the outbound sequence forward is always safe; forcing it backward, or forcing the inbound sequence to an arbitrary value, is intentionally permitted for chaos-testing scenarios but will likely desync you from a real counterparty - expect a disconnect or rejected messages afterward if you do this against anything other than a test harness.

### What's the deterministic scripting model - how do `wait`/`expect`/`assert` behave on timeout?

Scripts are deterministic: `wait`/`expect` block until a matching message arrives or the configured timeout elapses, and `assert` fails immediately if its condition is false. Any failure exits the script (and, in `mxshell -f`, exits the process with a non-zero status).

Think of `wait` as a barrier: it blocks until either the timeout fires or a matching message shows up anywhere in the incoming stream. `expect` is stricter - it requires the *very next* message to match, and fails immediately if anything else arrives first. When in doubt, prefer `wait`; `expect` is easy to trip up with an unrelated heartbeat or admin message landing in between.

### Can I run the same script from both MXGUI and MXShell with identical behavior?

Yes - both frontends invoke the exact same executor/session engine, so a script behaves identically whether run interactively in MXGUI's Script Runner or headlessly via `mxshell -f`.

### How do I use MicroFIX in a CI pipeline?

See the [Continuous Integration](README.md#continuous-integration) section in the main README - install `mxshell` via `go install` and run `mxshell -f your_script.mxs`; a failed `wait`/`expect`/`assert` fails the build automatically.

---

## GUI-Specific

### How do I switch between Light and Dark themes, and is the choice persisted?

Toggle it from the Settings/About panel. The choice is saved to the browser's local storage, so it persists across restarts.

### What's the difference between the Live Session Monitor and the offline Toolbox?

The Live Session Monitor streams real-time logs for an active connected session. The Toolbox works fully offline on pasted raw FIX text - useful for finalizing (BodyLength/CheckSum), validating, or decoding messages without a live connection.

### How do I compare two messages side-by-side (Message Diff)?

Open the Message Inspector and select two messages (from the live log or Toolbox) to diff - differing tags are highlighted automatically.

### How do I browse tag/field/message definitions in the Dictionary Browser?

Open the Dictionary page and search by tag number, field name, or message type - it reads directly from your configured `SessionSpec`/`ApplicationSpec` XML.

### How do I quickly jump back to a specific message in a long-running session log?

The log search bar has two modes: **Filter** hides everything that doesn't match your regex, while **Jump** keeps the full log visible and steps you through matches one at a time (with Enter/Shift+Enter or the arrow buttons), showing a running match count so you always know where you are.

### I have a message open in the Inspector but closed the log panel - how do I get back to it in context?

Reopen the Inspector for that message and use the **Locate** button - it clears any active filters and scrolls the log back to that exact message, briefly highlighting it so it's easy to spot among its neighbors.

### Can I reuse a message I already sent as the starting point for a new or edited alias?

Yes - when saving an alias whose name already exists, the check will offer a **Reload Payload** option, letting you pull in the live message content instead of retyping the template from scratch.

### Can I build a message field-by-field instead of typing raw FIX text?

Yes - the **Form Builder** (available from the send form toolbar) lets you add, remove, drag-reorder, and edit tag/value pairs individually, then apply the assembled result back into the message editor. It's useful when you want to construct a message without needing to remember exact tag numbers or delimiter syntax by hand.

### Can I stop a script while it's running?

Yes - the Script Runner shows a **Stop Execution** button whenever a script is running (or press **Esc**). This cancels the in-flight script the same way a failed `wait`/`expect`/`assert` would, without needing to close the whole session.

### What does the "Verbose Engine Logs" toggle do in the Script Runner?

When enabled, the console interleaves your script's own `print` output with the underlying session's admin-level traffic (Logons, Heartbeats, Test Requests, Sequence Resets, etc.) as it happens - useful for understanding exactly what the engine did behind the scenes during a run, not just what your script explicitly printed. Turn it off for cleaner output when you only care about your script's own messages.

---

## FIX Protocol

### Does MicroFIX automatically handle FIX session messages?

Yes. The protocol engine natively handles standard session-level behavior including Logon, Logout, Heartbeats, Test Requests, and Sequence Resets.

### Can I paste raw FIX messages?

Yes. Messages such as `35=D|55=AAPL|54=1|38=100|40=2|` can be pasted directly into MicroFIX. The engine can normalize delimiters, compute `BodyLength`, and calculate `CheckSum` on the fly.

### Do MXGUI and MXShell use different FIX implementations?

No. Both use the exact same parser, validator, message model, session engine, and scripting engine. A FIX flow debugged interactively in MXGUI uses the exact same underlying protocol implementation that will execute it from MXShell in CI.

---

## Troubleshooting

### My session won't connect / stays in a reconnect loop - what should I check first?

Verify `IpAddr`/`Port` and Sender/TargetCompID match what the counterparty expects, and confirm nothing else is bound to the port if you're listening (acceptor mode). Check the Live Session Monitor / `logs` output for rejected Logon messages, which usually indicate a CompID or sequence number mismatch.

Sometimes the rejection reason returned isn't detailed enough to diagnose from your side alone - if the mismatch isn't obvious from the logs, it's worth confirming directly with the counterparty what they expected to see.

### Why is my message failing strict validation?

Strict validation rejects unknown fields and enforces field data types against your XML dictionary. Check the validation error for the offending tag, and confirm it's actually defined in your `ApplicationSpec`/`SessionSpec` XML for that message type. If your venue diverges from spec, set `FixValidateStrict: false` to fall back to Basic-level checks.

### How do I export/search logs for a specific message or tag value?

Use the regex search in the Live Stream log view, or the `logs` command in MXShell for CLI-based filtering/export.

### Where do I report bugs or request features?

Open an issue or pull request on the [GitHub repo](https://github.com/Infinage/microfix).
