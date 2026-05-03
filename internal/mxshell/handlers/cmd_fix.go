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

	// Fields
	fmt.Println("  Fields")
	fmt.Println("  ------")
	fCount := 0
	for tag, field := range s.Spec().Fields {
		if re.MatchString(field.Name) || re.MatchString(strconv.Itoa(int(tag))) {
			fmt.Printf("  %-5d │ %s\n", tag, field.Name)
			fCount++
		}
	}
	if fCount == 0 {
		fmt.Println("  (no matches)")
	}

	// Messages
	fmt.Println("\n  Messages")
	fmt.Println("  --------")
	mCount := 0
	for msgType, msgDef := range s.Spec().Messages {
		if re.MatchString(msgDef.Name) || re.MatchString(msgType) {
			fmt.Printf("  %-5s │ %s\n", msgType, msgDef.Name)
			mCount++
		}
	}
	if mCount == 0 {
		fmt.Println("  (no matches)")
	}

	fmt.Println("\n────────────────────────────────────────────────────")
	fmt.Printf("  Found: %d fields, %d messages\n", fCount, mCount)
	fmt.Println("────────────────────────────────────────────────────")
}

func queryFixSpec(ctx *AppContext, sub, id string) {
	sp := ctx.Session.Spec()

	switch sub {
	case "field":
		tag, _ := strconv.Atoi(id)
		if f, ok := sp.Fields[uint16(tag)]; ok {
			fmt.Println("\n─── Field Definition ──────────────────────────────")
			pretty.FieldDef(os.Stdout, f)
			fmt.Println("────────────────────────────────────────────────────")
		} else {
			fmt.Printf("Field %s not found\n", id)
		}
	case "message":
		if m, ok := sp.Messages[id]; ok {
			fmt.Println("\n─── Message Definition ────────────────────────────")
			pretty.SpecEntry(os.Stdout, m, sp.FieldNames, ctx.Config.SpecDisplayOptFields, 0)
			fmt.Println("────────────────────────────────────────────────────")
		} else {
			fmt.Printf("Message %s not found\n", id)
		}
	case "sample":
		// If ValidationStrict is false, do a basic validation only
		includeOptional := false
		if ctx.Config.FixSampleOptional {
			includeOptional = true
		}

		if smp, err := sp.Sample(id, spec.SampleOptions{IncludeOptional: includeOptional}); err == nil {
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

// Prettify and print the output matching against fix spec, does not validate
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
	pretty.Message(os.Stdout, &msg, ctx.Session.Spec())
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

	// If ValidationStrict is false, do a basic validation only
	validationMode := spec.ValidationStrict
	if !ctx.Config.FixValidateStrict {
		validationMode = spec.ValidationBasic
	}

	_, obs := ctx.Session.Spec().Validate(&msg, validationMode)

	if len(obs) == 0 {
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
	sp := ctx.Session.Spec()

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

	fmt.Printf("\n──── %s Definition ────────────────────────────", strings.ToUpper(meta))
	pretty.SpecEntry(os.Stdout, entry, sp.FieldNames, ctx.Config.SpecDisplayOptFields, 0)
	fmt.Println("────────────────────────────────────────────────────")
}

func handleFix(ctx *AppContext, args []string) {
	if len(args) < 3 || len(args[2]) == 0 {
		fmt.Println("Usage: \n" +
			"fix meta [header|trailer]\n" +
			"fix [field|message|sample] id\n" +
			"fix validate <fixMessage>\n" +
			"fix search pattern")
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
		"fix [search <regex> | meta <header|trailer> | <decode|validate> <msg> | <field|message|sample> <id>]",
	)
}
