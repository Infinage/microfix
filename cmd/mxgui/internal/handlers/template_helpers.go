package gui

import "github.com/infinage/microfix/pkg/store"

func getSpecName(config store.Config) string {
	if config.ApplicationSpec != "" {
		return config.ApplicationSpec
	}
	return config.SessionSpec
}

func getColorName(state string) string {
	switch state {
	case "New":
		return "blue"
	case "Listening", "LoggingIn":
		return "emerald"
	case "Connected":
		return "green"
	case "Stale":
		return "yellow"
	case "Closed":
		return "amber"
	default:
		return "red"
	}
}
