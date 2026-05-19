package executor

import (
	"strings"
	"sync"
	"testing"

	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
)

// Utility to dump logs from channel into our slice of strings
func dumpLogs(logs *[]string, logCh <-chan session.Log, router *spec.Router) {
	for log := range logCh {
		hint := ""
		if log.Type == session.LogSend || log.Type == session.LogRecv {
			msgType, _ := log.Msg.Get(35)
			entry, ok := router.SpecForMsgType(msgType).Messages[msgType]
			if ok {
				hint = entry.Name
			}
		}
		*logs = append(*logs, log.String(hint))
	}
}

type e2eResult struct {
	clientOut  string
	serverOut  string
	clientLogs []string
	serverLogs []string
}

// Starts two sessions and have them interact with each other via scripts
func runE2E(t *testing.T, specVer, clientScript, serverScript string) e2eResult {
	t.Helper()

	// Create seperate sessions for server and client
	clientSess, clientSessErr := session.NewSession(specVer, "CLIENT", "SERVER", 5, session.EngineOptions{})
	if clientSessErr != nil {
		t.Fatalf("Failed to create session with '%v': %v", specVer, clientSessErr)
	}
	serverSess, serverSessErr := session.NewSession(specVer, "SERVER", "CLIENT", 5, session.EngineOptions{})
	if serverSessErr != nil {
		t.Fatalf("Failed to create session with '%v': %v", specVer, serverSessErr)
	}

	clientCtx, clientOut := setupTestContext(t, clientSess)
	serverCtx, serverOut := setupTestContext(t, serverSess)

	clientLogCh, closeClientLogs, clientLogErr := clientSess.SubscribeLog()
	if clientLogErr != nil {
		t.Fatal("Failed to subscribe to log for client")
	}
	serverLogCh, closeServerLogs, serverLogErr := serverSess.SubscribeLog()
	if serverLogErr != nil {
		t.Fatal("Failed to subscribe to log for client")
	}

	var clientLogs, serverLogs []string
	var wg sync.WaitGroup

	// Log dumpers
	wg.Go(func() { dumpLogs(&clientLogs, clientLogCh, clientSess.Router()) })
	wg.Go(func() { dumpLogs(&serverLogs, serverLogCh, serverSess.Router()) })

	// Run Server side script
	wg.Go(func() {
		defer closeServerLogs()
		if err := EvalBatch(strings.NewReader(serverScript), serverCtx); err != nil {
			t.Errorf("Server script failed: %v", err)
		}
	})

	// Run Client side script
	wg.Go(func() {
		defer closeClientLogs()
		if err := EvalBatch(strings.NewReader(clientScript), clientCtx); err != nil {
			t.Errorf("Client script failed: %v", err)
		}
	})

	// Wait for scripts and logs to finish
	wg.Wait()

	return e2eResult{
		clientOut:  clientOut.String(),
		serverOut:  serverOut.String(),
		clientLogs: clientLogs,
		serverLogs: serverLogs,
	}
}

// runNativeE2E spins up a native Go server session and runs a client script against it.
func runNativeE2E(t *testing.T, specVer, addr, clientScript string) e2eResult {
	t.Helper()

	// Create sessions
	clientSess, _ := session.NewSession(specVer, "CLIENT", "SERVER", 5, session.EngineOptions{})
	serverSess, _ := session.NewSession(specVer, "SERVER", "CLIENT", 5, session.EngineOptions{})

	// Setup Client Context
	clientCtx, clientOut := setupTestContext(t, clientSess)

	clientLogCh, closeClientLogs, clientLogErr := clientSess.SubscribeLog()
	if clientLogErr != nil {
		t.Fatal("Failed to subscribe to log for client")
	}
	serverLogCh, closeServerLogs, serverLogErr := serverSess.SubscribeLog()
	if serverLogErr != nil {
		t.Fatal("Failed to subscribe to log for client")
	}

	var clientLogs, serverLogs []string
	var wg sync.WaitGroup

	// Log dumpers
	wg.Go(func() { dumpLogs(&clientLogs, clientLogCh, clientSess.Router()) })
	wg.Go(func() { dumpLogs(&serverLogs, serverLogCh, serverSess.Router()) })

	// Start Native Server
	wg.Go(func() {
		if err := serverSess.Listen(addr); err != nil {
			t.Errorf("Native server listen failed: %v", err)
		}
	})

	// Run Client Script
	wg.Go(func() {
		defer closeServerLogs()
		defer closeClientLogs()
		if err := EvalBatch(strings.NewReader(clientScript), clientCtx); err != nil {
			t.Errorf("Client script failed: %v", err)
		}
	})

	// Wait for scripts and logs to finish
	wg.Wait()

	return e2eResult{
		clientOut:  clientOut.String(),
		serverOut:  "Native Server (No Script Output)",
		clientLogs: clientLogs,
		serverLogs: serverLogs,
	}
}

func TestEvalBatch_LogonAndPing(t *testing.T) {
	// Define the Server Script (Acceptor)
	serverScript := `
# Set a timeout (independent of cfg)
set CFG.DefaultTimeoutSec 3

listen 127.0.0.1:5005
print Connected to Client, Session: $STATUS!

# Wait for client logon
wait 35=A
print Server received Logon

# Validate engine status as Active
assert $STATUS Connected

# Wait for client to send a test request
wait 35=1
print Server received TestRequest: $LASTIN[1, 112]

# No need to manually send since engine auto responds with heartbeat
# A bit of timeout for heartbeat to be sent to client
sleep 10

# Close the connection and check status as Closed
disconnect
assert $STATUS Closed
`

	// Define the Client Script (Initiator)
	clientScript := `
# Give server a tiny fraction of a second to bind to the port
sleep 10

# Set a timeout (independent of cfg)
set CFG.DefaultTimeoutSec 3

connect 127.0.0.1:5005
print Connected to server, Session: $STATUS!

# Wait for server logon ack
wait 35=A
print Client received Logon

# Send a TestRequest and wait for Heartbeat
send 8=FIX.4.4|9=68|35=1|49=STRING|56=STRING|34=704|52=20260519-11:13:39.976|112=PING|10=173|
wait 35=0 & 112=PING
print Client received Heartbeat!

# Close the connection and check status as Closed
disconnect
assert $STATUS Closed
`
	// Run test harness
	res := runE2E(t, "FIX44.xml", clientScript, serverScript)

	// Helpful for debugging if tests fail
	t.Log("\n--- Server Logs ---\n" + strings.Join(res.serverLogs, "\n"))
	t.Log("\n--- Client Logs ---\n" + strings.Join(res.clientLogs, "\n"))
}

func TestEvalBatch_NativeServer_SequenceGap(t *testing.T) {
	addr := "127.0.0.1:5006"

	clientScript := `
sleep 10
set CFG.DefaultTimeoutSec 5

connect ` + addr + `
wait 35=A

# Force a sequence gap (Jumping our outbound to MsgSeqNum 5)
seq out 5

# Engine auto triggers a Sequence Reset to Server
# Subsequent messages should work just fine
assert $SEQ_OUT 5

# Send a TestRequest. The engine will send it as MsgSeqNum 5.
send 8=FIX.4.4|9=68|35=1|49=CLIENT|56=SERVER|34=5|52=20260519-11:13:39|112=PING|10=173|

wait 35=0 & 112=PING
print Client received Heartbeat after GapFill recovery

disconnect
`
	res := runNativeE2E(t, "FIX44.xml", addr, clientScript)

	if !strings.Contains(res.clientOut, "GapFill recovery") {
		t.Errorf("Sequence Gap test failed. Output:\n%s", res.clientOut)
	}

	// Helpful for debugging if tests fail
	t.Log("\n--- Server Logs ---\n" + strings.Join(res.serverLogs, "\n"))
	t.Log("\n--- Client Logs ---\n" + strings.Join(res.clientLogs, "\n"))
}

func TestEvalBatch_NativeServer_SequenceTooLow(t *testing.T) {
	addr := "127.0.0.1:5007"

	clientScript := `
sleep 10
set CFG.DefaultTimeoutSec 3

connect ` + addr + `
wait 35=A

# Artificially lower our outbound sequence number back to 1 (Fatal Error)
seq out 1

# Send a standard message with the invalid sequence number
send 8=FIX.4.4|9=68|35=1|49=CLIENT|56=SERVER|34=1|52=20260519-11:13:39|112=PING|10=173|

# The native server should instantly detect the violation and kick us out
sleep 10
assert $STATUS Closed
print Client received Fatal Logout!
`
	res := runNativeE2E(t, "FIX44.xml", addr, clientScript)

	if !strings.Contains(res.clientOut, "Client received Fatal Logout!") {
		t.Errorf("Server failed to issue Logout for low sequence. Output:\n%s", res.clientOut)
	}

	// Helpful for debugging if tests fail
	t.Log("\n--- Server Logs ---\n" + strings.Join(res.serverLogs, "\n"))
	t.Log("\n--- Client Logs ---\n" + strings.Join(res.clientLogs, "\n"))
}
