package handlers

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/infinage/microfix/internal/mxshell/config"
	"github.com/infinage/microfix/internal/mxshell/pretty"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
)

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

func handleFixSpecQuery(s *session.Session, cfg *config.Config, args []string) {
	sub := strings.ToLower(args[0])
	id := args[1]

	switch sub {
	case "field":
		tag, _ := strconv.Atoi(id)
		if f, ok := s.Spec().Fields[uint16(tag)]; ok {
			pretty.WritePrettyFieldDef(os.Stdout, f)
		} else {
			fmt.Printf("Field %s not found\n", id)
		}
	case "message":
		if m, ok := s.Spec().Messages[id]; ok {
			pretty.WritePrettySpecEntry(os.Stdout, m, s.Spec().FieldNames, cfg.SpecDisplayOptFields, 0)
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

func handleFix(ctx *AppContext, args []string) {
	if len(args) < 3 {
		fmt.Println("Usage: fix [field|message|sample|search] <id/pattern>")
		return
	}

	if strings.ToLower(args[1]) == "search" {
		handleFixSearch(ctx.Session, args[2])
	} else {
		handleFixSpecQuery(ctx.Session, ctx.Config, args[1:])
	}
}

func init() {
	RegisterCommandHandler("fix", handleFix)
}
