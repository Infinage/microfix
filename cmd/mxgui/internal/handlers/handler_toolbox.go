package gui

import (
	"fmt"
	"net/http"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

func (app *Application) handleAPIFinalize(w http.ResponseWriter, r *http.Request) {
	msgRaw := r.URL.Query().Get("finalize-input")
	if len(msgRaw) < 4 {
		toast(w, app.templ, "error", "Input must be atleast 4 chars long")
		return
	}

	delim := msgRaw[len(msgRaw)-1:]
	msg, err := message.MessageFromString(msgRaw, delim)
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Invalid fix string input: %s", err.Error()))
		return
	}

	// Finalize the message and return fragment
	msg.Finalize()
	w.Write([]byte(msg.String(delim)))
}

func (app *Application) handleAPIValidate(w http.ResponseWriter, r *http.Request) {
	msgRaw := r.URL.Query().Get("validate-input")
	if len(msgRaw) < 4 {
		renderTemplate(app.templ, w, "partials/toolbox/validate/report", []string{"Structural Error: Input must be at least 4 chars long"})
		return
	}

	// Try to parse the structural FIX message
	delim := msgRaw[len(msgRaw)-1:]
	msg, err := message.MessageFromString(msgRaw, delim)
	if err != nil {
		errMsg := fmt.Sprintf("Structural Error: Invalid fix string input - %s", err.Error())
		renderTemplate(app.templ, w, "partials/toolbox/validate/report", []string{errMsg})
		return
	}

	// Spec Dictionary Validation
	result, _ := app.Session.Router().Validate(&msg, spec.ValidationStrict)
	renderTemplate(app.templ, w, "partials/toolbox/validate/report", result)
}
