package handlers

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/pretty"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
)

func searchFixSpec(s *session.Session, pattern string) {
	fmt.Println("\n─── FIX Spec Search ───────────────────────────────")
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("────────────────────────────────────────────────────")
		return
	}

	fmt.Printf("  Pattern : %s\n\n", pattern)

	ro := s.Router()
	sessSpec := ro.SessionSpec()
	applSpec := ro.ApplSpec()

	// Merge Session and Appl results so user sees both
	fmt.Println("  Fields")
	fmt.Println("  ------")
	fCount := 0

	// Helper to search a field map
	searchFields := func(fields map[uint16]spec.FieldDef, source string) {
		for tag, field := range fields {
			if re.MatchString(field.Name) || re.MatchString(strconv.Itoa(int(tag))) {
				fmt.Printf("  %-5d │ %-25s [%s]\n", tag, field.Name, source)
				fCount++
			}
		}
	}

	searchFields(sessSpec.Fields, "Session")
	if sessSpec != applSpec {
		searchFields(applSpec.Fields, "App")
	}

	if fCount == 0 {
		fmt.Println("  (no matches)")
	}

	// Search Messages
	fmt.Println("\n  Messages")
	fmt.Println("  --------")
	mCount := 0

	searchMessages := func(messages map[string]spec.Entry, source string) {
		for msgType, msgDef := range messages {
			if re.MatchString(msgDef.Name) || re.MatchString(msgType) {
				fmt.Printf("  %-5s │ %-25s [%s]\n", msgType, msgDef.Name, source)
				mCount++
			}
		}
	}

	searchMessages(sessSpec.Messages, "Session")
	if sessSpec != applSpec {
		searchMessages(applSpec.Messages, "App")
	}

	if mCount == 0 {
		fmt.Println("  (no matches)")
	}

	fmt.Println("\n────────────────────────────────────────────────────")
	fmt.Printf("  Found: %d fields, %d messages\n", fCount, mCount)
	fmt.Println("────────────────────────────────────────────────────")
}

func queryFixSpec(ctx *AppContext, sub, id string) {
	ro := ctx.Session.Router()
	cfg := ctx.Store.Config()

	switch sub {
	case "field":
		tag, _ := strconv.Atoi(id)

		// Try Session Spec first, then App Spec
		f, ok := ro.SessionSpec().Fields[uint16(tag)]
		if !ok {
			f, ok = ro.ApplSpec().Fields[uint16(tag)]
		}

		if ok {
			fmt.Println("\n─── Field Definition ──────────────────────────────")
			pretty.FieldDef(os.Stdout, f)
			fmt.Println("────────────────────────────────────────────────────")
		} else {
			fmt.Printf("Field %s not found\n", id)
		}

	case "message":
		// Ask the router which spec owns this message
		msgSpec := ro.SpecForMsgType(id)
		if m, ok := msgSpec.Messages[id]; ok {
			fmt.Println("\n─── Message Definition ────────────────────────────")
			pretty.SpecEntry(os.Stdout, m, msgSpec.FieldNames, cfg.SpecDisplayOptFields, 0)
			fmt.Println("────────────────────────────────────────────────────")
		} else {
			fmt.Printf("Message %s not found\n", id)
		}

	case "sample":
		includeOptional := false
		if cfg.FixSampleOptional {
			includeOptional = true
		}

		// The Router seamlessly stitches the sample together!
		if smp, err := ro.Sample(id, spec.SampleOptions{IncludeOptional: includeOptional}); err == nil {
			fmt.Println("\n─── Sample Message ────────────────────────────────")
			fmt.Println(smp.String("|"))
			fmt.Println("────────────────────────────────────────────────────")
		} else {
			fmt.Println("Sampling failed:", err)
		}
	default:
		fmt.Println("2nd argument must be one of field, message, sample")
	}
}

// Prettify and print the output matching against fix spec
func decodeMessage(ctx *AppContext, rawMsg string) {
	delim := rawMsg[len(rawMsg)-1:]
	msg, err := message.MessageFromString(rawMsg, delim)
	if err != nil {
		fmt.Println("\n─── Decode Message ────────────────────────────────")
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("────────────────────────────────────────────────────")
		return
	}

	fmt.Println("\n─── FIX Message (Spec View) ────────────────────────")
	pretty.Message(os.Stdout, &msg, ctx.Session.Router())
	fmt.Println("────────────────────────────────────────────────────")
}

// Recomputes the checksum and bodylen - inserts if missing
func finalizeRawMessage(_ *AppContext, rawMsg string) {
	delim := rawMsg[len(rawMsg)-1:]
	msg, err := message.MessageFromString(rawMsg, delim)
	if err != nil {
		fmt.Println("\n─── Finalize Message ──────────────────────────────")
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("────────────────────────────────────────────────────")
		return
	}
	fmt.Println("\n─── Finalized Message ─────────────────────────────────")
	msg.Finalize()
	fmt.Println(msg.String(delim))
	fmt.Println("────────────────────────────────────────────────────")
}

func validateMessage(ctx *AppContext, rawMsg string) {
	fmt.Println("\n─── FIX Validation ────────────────────────────────")

	delim := rawMsg[len(rawMsg)-1:]
	msg, err := message.MessageFromString(rawMsg, delim)
	if err != nil {
		fmt.Printf("  Status : FAILED\n")
		fmt.Printf("  Error  : %v\n", err)
		fmt.Println("────────────────────────────────────────────────────")
		return
	}

	validationMode := spec.ValidationStrict
	if !ctx.Store.Config().FixValidateStrict {
		validationMode = spec.ValidationBasic
	}

	obs, ok := ctx.Session.Router().Validate(&msg, validationMode)

	if ok {
		fmt.Println("  Status : OK")
		fmt.Println("────────────────────────────────────────────────────")
		return
	}

	fmt.Print("  Status : FAILED\n\n")
	fmt.Println("  Issues")
	fmt.Println("  ------")

	for _, ob := range obs {
		fmt.Printf("  • %s\n", ob)
	}

	fmt.Println("────────────────────────────────────────────────────")
	fmt.Printf("  Total Issues: %d\n", len(obs))
	fmt.Println("────────────────────────────────────────────────────")
}

func displayMeta(ctx *AppContext, meta string) {
	// Headers and Trailers ALWAYS belong to the Session Spec
	sp := ctx.Session.Router().SessionSpec()

	var entry spec.Entry
	switch meta {
	case "header":
		entry = sp.Header
	case "trailer":
		entry = sp.Trailer
	default:
		fmt.Printf("Must be one of 'header', 'trailer', got: %v\n", meta)
		return
	}

	fmt.Printf("\n──── %s Definition ────────────────────────────\n", strings.ToUpper(meta))
	pretty.SpecEntry(os.Stdout, entry, sp.FieldNames, ctx.Store.Config().SpecDisplayOptFields, 0)
	fmt.Println("────────────────────────────────────────────────────")
}
func handleFix(ctx *AppContext, args []string) {
	if len(args) < 3 || len(args[2]) == 0 {
		fmt.Println("Usage: \n" +
			"fix meta [header|trailer]\n" +
			"fix [field|message|sample] <id>\n" +
			"fix [decode|validate|finalize] <fixMessage>\n" +
			"fix search <pattern>")
		return
	}

	if sub := strings.ToLower(args[1]); sub == "search" {
		searchFixSpec(ctx.Session, args[2])
	} else if sub == "meta" {
		displayMeta(ctx, args[2])
	} else if sub == "validate" {
		validateMessage(ctx, args[2])
	} else if sub == "decode" {
		decodeMessage(ctx, args[2])
	} else if sub == "finalize" {
		finalizeRawMessage(ctx, args[2])
	} else {
		sub := strings.ToLower(args[1])
		id := args[2]
		queryFixSpec(ctx, sub, id)
	}
}

func init() {
	RegisterCommand(
		"fix",
		handleFix,
		"Query FIX dictionary, generate samples, and validate messages",
		"fix [search <regex> | meta <header|trailer> | <decode|validate|finalize> <msg> | <field|message|sample> <id>]",
	)
}
