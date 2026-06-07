package gui

import (
	"github.com/infinage/microfix/pkg/store"
)

// struct to hold the theme colors
type Theme struct {
    Text string
    Bg   string
	Border string
}

// Return appl spec if available else session spec
func getSpecName(config store.Config) string {
	if config.ApplicationSpec != "" {
		return config.ApplicationSpec
	}
	return config.SessionSpec
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
	case "Connected":
		textColor = "text-green-400"
		bgColor = "bg-green-500"
	case "Stale":
		textColor = "text-yellow-400"
		bgColor = "bg-yellow-500"
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
