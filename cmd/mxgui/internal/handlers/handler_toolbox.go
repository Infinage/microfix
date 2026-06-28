package gui

import (
	"fmt"
	"net/http"
	"time"

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

	// Populate a slice of mandatory field info
	// we will attempt to salvage
	type MandatoryField struct {
		tag   uint16
		value string
		pos   int
	}

	// critical tags: [8, 9, 35, 49, 56, 34, 52, 10]
	ro := app.Session().Router()
	beginStr := ro.SessionSpec().BeginString()
	sendingTime := time.Now().UTC().Format("20060102-15:04:05.000")
	for _, rf := range []MandatoryField{
		{tag: 8, value: beginStr, pos: 0},
		{tag: 9, value: "", pos: 1},
		{tag: 35, value: "0", pos: 2},
		{tag: 49, value: "FROM", pos: 3},
		{tag: 56, value: "TO", pos: 4},
		{tag: 34, value: "1", pos: 5},
		{tag: 52, value: sendingTime, pos: 6},
	} {
		if _, ok := msg.Get(rf.tag); !ok {
			msg.Insert(rf.pos, message.Field{Tag: rf.tag, Value: rf.value})
		}
	}

	msg.Finalize() // Auto inserts tag 9 and 10 if missing
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
	result, _ := app.Session().Router().Validate(&msg, spec.ValidationStrict)
	renderTemplate(app.templ, w, "partials/toolbox/validate/report", result)
}
