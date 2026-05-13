/*
To be used in a SINGLE THREADED context only
All calls to engine public APIs must be routed through session's run loop
*/

package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

type SessionState int

const (
	SessionNew SessionState = iota
	SessionListening
	SessionLoggingIn
	SessionActive
	SessionStale
	SessionClosed
)

func (s SessionState) String() string {
	names := []string{"New", "Listening", "Logging In", "Connected", "Stale", "Closed"}
	if s < 0 || int(s) >= len(names) {
		return "Unknown"
	}
	return names[s]
}

type Snapshot struct {
	State         SessionState
	InSeqNum      int64
	OutSeqNum     int64
	LastReadTime  time.Time
	LastWriteTime time.Time
}

// Holds optional configuration for the session engine.
// The zero-value of this struct represent safe defaults.
type EngineOptions struct {
	// DefaultApplVerID sets the wire value for Tag 1137 (e.g., "7" for FIX 5.0).
	// Required if using FIXT multiplexing.
	DefaultApplVer string

	// Skip latency check - by default messages with time delta exceeding 2 min are rejected
	SkipLatencyCheck bool
}

type Engine struct {
	Router spec.Router
	state  SessionState

	senderCompID string
	targetCompID string
	heartbeatInt int64
	testReqID    string

	inSeqNum  int64
	outSeqNum int64

	lastWriteTime time.Time
	lastReadTime  time.Time

	store MessageStore

	extraOpts EngineOptions
}

// Custom error type to send message rejects
type RejectError struct {
	RefSeqNum int64
	Text      string
}

func (err *RejectError) Error() string {
	return fmt.Sprintf("[%v] - %v", err.RefSeqNum, err.Text)
}

func NewEngine(router *spec.Router, senderCompID string, targetCompID string, heartbeatInt int64, opts EngineOptions) (*Engine, error) {
	if heartbeatInt <= 0 {
		return nil, fmt.Errorf("Heartbeat Interval must be greater than 0")
	}

	// Attempt to load the spec
	if router == nil {
		return nil, fmt.Errorf("Router cannot be nil")
	}

	// Set the defaultApplVer from EngineOptions if provided
	if ver := opts.DefaultApplVer; ver != "" && !router.SetDefaultApplVer(ver) {
		return nil, fmt.Errorf("Failed to set DefaultApplVer: %v", ver)
	}

	// Ensure spec contains the message def for what we will be sending
	for _, msgType := range []string{"0", "1", "2", "3", "4", "5", "A"} {
		if _, err := router.Sample(msgType, spec.SampleOptions{}); err != nil {
			return nil, fmt.Errorf("Failed to sample message: %v", msgType)
		}
	}

	engine := &Engine{
		Router:       *router,
		senderCompID: senderCompID,
		targetCompID: targetCompID,
		heartbeatInt: heartbeatInt,
		testReqID:    "MICROFIX",
		inSeqNum:     1,
		outSeqNum:    1,
		store:        NewMessageStore(),
		extraOpts:    opts,
	}

	// Starting as New State
	engine.state = SessionNew

	return engine, nil
}

func (engine *Engine) Snapshot() Snapshot {
	return Snapshot{
		State:         engine.state,
		InSeqNum:      engine.inSeqNum,
		OutSeqNum:     engine.outSeqNum,
		LastReadTime:  engine.lastReadTime,
		LastWriteTime: engine.lastWriteTime,
	}
}

// Feedback that a write action was successful
func (engine *Engine) recordWrite(msg *message.Message, now time.Time) {
	engine.lastWriteTime = now

	if msgType, _ := msg.Get(35); msgType != "4" {
		// Ignore updating outSeqNum increment for a SequenceReset
		// would be set by `OnResetSequence` instead
		engine.outSeqNum++

		// Store only non admin message into our store
		// And we clone so that downstream updates dont affect the store
		if !engine.Router.IsAdmin(msgType) {
			engine.store.Append(*msg)
		}
	}
}

// Returns an error if missing tags: [8, 9, 35, 49, 56, 34, 52, 10] or on failing outbound validation.
func (engine *Engine) finalizeMessage(msg *message.Message, now time.Time) error {
	if !msg.Contains(8, 9, 35, 49, 56, 34, 52, 10) {
		return fmt.Errorf("OUTBOUND missing required session fields: %s", msg.String("|"))
	}

	// Set sender / target compId
	msg.Set(49, engine.senderCompID)
	msg.Set(56, engine.targetCompID)

	// Update OutSeqNum only if NOT a fill gap
	msgType, _ := msg.Get(35)
	gapFillFlag, _ := msg.Get(123)
	if !(msgType == "4" && gapFillFlag == "Y") {
		msg.Set(34, fmt.Sprint(engine.outSeqNum))
	}

	// Set SendingTime
	msg.Set(52, now.UTC().Format("20060102-15:04:05.000"))

	// Enforce session BeginString
	msg.Set(8, engine.Router.SessionSpec().BeginString())

	// Recalculate the bodylen and checksum
	msg.Finalize()

	// Perform basic validate before sending
	if obs, ok := engine.validateAgainstSpec(msg, spec.ValidationBasic); !ok {
		return fmt.Errorf("OUTBOUND validation failed: %v | Message: %s", obs, msg.String("|"))
	}

	return nil
}

// Helper to temporarily switch engine DefaultApplVerID if applVerID is set, validate and toggle back
func (engine *Engine) validateAgainstSpec(msg *message.Message, mode spec.ValidationMode) ([]string, bool) {
	beginStr, ok := msg.Get(8)
	if !ok {
		return []string{"Missing BeginString"}, false
	}

	// Switch DefaultApplVerID if ApplVerID is set
	oldApplVer := engine.Router.GetDefaultApplVerID()
	if applVerID, ok := msg.Get(1128); ok && strings.HasPrefix(beginStr, "FIXT") {
		if !engine.Router.SetDefaultApplVerID(applVerID) {
			return []string{fmt.Sprintf("Message specifies unsupported ApplVerID [1128]: %v", applVerID)}, false
		}
		defer engine.Router.SetDefaultApplVerID(oldApplVer)
	}

	// Validate per input spec
	return engine.Router.Validate(msg, mode)
}

// Checks for MsgType, TargetCompID, SenderCompID, InSeqNum, SendingTime
// followed by Spec validation including Checksum, BodyLength
func (engine *Engine) validate(msg *message.Message, now time.Time) error {
	// Validate BeginString [8]
	beginStr, ok := msg.Get(8)
	if want := engine.Router.SessionSpec().BeginString(); !ok || beginStr != want {
		return fmt.Errorf("BeginString mistmatch, expected %v, found %v", want, beginStr)
	}

	// Validate MsgType [35]
	msgType, ok := msg.Get(35)
	if !ok {
		return fmt.Errorf("Missing MsgType tag [35]")
	}

	// Validate the TargetCompID / SenderCompID: check is swapped
	if senderCompID, _ := msg.Get(49); senderCompID != engine.targetCompID {
		return fmt.Errorf("SenderCompID [49] mismatch, expected '%v' got '%v'", engine.targetCompID, senderCompID)
	}
	if targetCompID, _ := msg.Get(56); targetCompID != engine.senderCompID {
		return fmt.Errorf("TargetCompID [56] mismatch, expected '%v' got '%v'", engine.senderCompID, targetCompID)
	}

	// Validate that message has InSeqNum, set default value guaranteed to fail
	var received int64 = 0
	if inSeqNumTag, seqNoPos := msg.FindFrom(34, 0); seqNoPos != -1 {
		if val, err := inSeqNumTag.AsInt(); err == nil {
			received = val
		}
	}

	// Skip sequence number check if:
	// 1. It's a logon with resetSeqNumFlag set
	// 2. It's a retransmitted message (PossDup)
	possDup, _ := msg.Get(43)
	resetSeqNum, _ := msg.Get(141)
	skipSeqCheck := (msgType == "A" && resetSeqNum == "Y") || possDup == "Y"

	// For values greater than we will trigger resend on `handleAppMessage`
	if !skipSeqCheck && received < engine.inSeqNum {
		return fmt.Errorf("Input sequence number mismatch. Expected %v, got %v", engine.inSeqNum, received)
	}

	// SendingTime validation - by passed if SkipLatencyCheck is set in EngineOpts
	if sendingTimeTag, pos := msg.FindFrom(52, 0); pos == -1 {
		return fmt.Errorf("Missing required session field SendingTime [52]")
	} else if sendingTime, err := sendingTimeTag.AsTZTimestamp(); err != nil {
		return &RejectError{RefSeqNum: engine.inSeqNum, Text: "Invalid SendingTime [52] value"}
	} else if diff := now.Sub(sendingTime).Abs(); !engine.extraOpts.SkipLatencyCheck && diff > 2*time.Minute {
		return &RejectError{RefSeqNum: engine.inSeqNum, Text: "SendingTime accuracy problem [52]"}
	}

	// Validate for FIX Spec correctness
	if obs, ok := engine.validateAgainstSpec(msg, spec.ValidationBasic); !ok {
		return &RejectError{
			RefSeqNum: engine.inSeqNum,
			Text:      fmt.Sprintf("Message validation failed: %v", obs),
		}
	}

	return nil
}
