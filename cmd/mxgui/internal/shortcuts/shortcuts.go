package shortcuts

import "encoding/json"

type shortcut struct {
	Key         string   `json:"key"`             // Eg: "Enter", "K", "Escape"
	Ctrl        bool     `json:"ctrl,omitempty"`  // Ctrl key
	Alt         bool     `json:"alt,omitempty"`   // Alt key
	Shift       bool     `json:"shift,omitempty"` // Shift key
	Events      []string `json:"events"`          // Events to bubble up to frontend listeners
	Description string   `json:"description"`     // Human readable description
}

var appShortcuts = []shortcut{
	{
		Key:         "Escape",
		Events:      []string{"close-modal", "close-inspector"},
		Description: "Close Inspector / Modal",
	},

	{
		Key:         "d",
		Alt:         true,
		Events:      []string{"view-dictionary"},
		Description: "Open FIX dictionary",
	},

	{
		Key:         "l",
		Alt:         true,
		Events:      []string{"view-stream"},
		Description: "Open live stream",
	},

	{
		Key:         "s",
		Alt:         true,
		Events:      []string{"view-settings"},
		Description: "Open settings",
	},
}

// Serialises appShortcuts defined to be used in the html views
func Shortcuts() string {
	b, err := json.Marshal(appShortcuts)
	if err != nil {
		return "[]"
	}
	return string(b)
}
