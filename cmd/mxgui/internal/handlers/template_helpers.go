package gui

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/store"
)

// struct to hold the theme colors
type Theme struct {
	Text   string
	Bg     string
	Border string
}

// Return appl spec if available else session spec
func getSpecName(config store.Config) string {
	sessSpec, appSpec := config.SessionSpec, config.ApplicationSpec

	sessSpec = strings.TrimSuffix(sessSpec, ".xml")
	appSpec = strings.TrimSuffix(appSpec, ".xml")

	if appSpec != "" {
		return fmt.Sprintf("%s [%s]", sessSpec, appSpec)
	}
	return sessSpec
}

func add2(n1, n2 int) int {
	return n1 + n2
}

func replaceSOH(raw string) string {
	return strings.ReplaceAll(raw, "\x01", "|")
}

func getThemeForEngineState(state string) Theme {
	textColor := ""
	bgColor := ""
	switch state {
	case "New":
		textColor = "text-blue-400"
		bgColor = "bg-blue-500"
	case "Listening", "LoggingIn":
		textColor = "text-emerald-400"
		bgColor = "bg-emerald-500"
	case "Active":
		textColor = "text-green-400"
		bgColor = "bg-green-500"
	case "Stale":
		textColor = "text-yellow-400"
		bgColor = "bg-yellow-500"
	case "OutOfSync":
		textColor = "text-orange-400"
		bgColor = "bg-orange-500"
	case "Closed":
		textColor = "text-gray-400"
		bgColor = "bg-gray-500"
	}

	return Theme{Text: textColor, Bg: bgColor}
}

func getThemeForLogType(state string) Theme {
	textColor := "text-green-500"
	borderColor := "border-l-green-500"

	switch state {
	case "SEND":
		textColor = "text-blue-500"
		borderColor = "border-l-blue-500"
	case "RECV":
		textColor = "text-green-500"
		borderColor = "border-l-green-500"
	case "ERR ":
		textColor = "text-red-500"
		borderColor = "border-l-red-500"
	case "SYS ":
		textColor = "text-yellow-500"
		borderColor = "border-l-yellow-500"
	}

	return Theme{Text: textColor, Border: borderColor}
}

func getAllFieldNamesAsJSON(r *spec.Router) template.JS {
	fields := make(map[uint16]string)
	for tag, field := range r.SessionSpec().Fields {
		fields[tag] = field.Name
	}

	if !r.IsLegacyRouter() {
		for tag, field := range r.ApplSpec().Fields {
			fields[tag] = field.Name
		}
	}

	tagMapJSON, err := json.Marshal(fields)
	if err != nil {
		return "{}"
	}

	return template.JS(tagMapJSON)
}
