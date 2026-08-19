package gui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

func (app *Application) handleAPIFinalize(w http.ResponseWriter, r *http.Request) {
	msgRaw := strings.TrimSpace(r.URL.Query().Get("finalize-input"))
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

	ro := app.Session().Router()
	msg = ro.Salvage(msg)

	w.Write([]byte(msg.String(delim)))
}

func (app *Application) handleAPIValidate(w http.ResponseWriter, r *http.Request) {
	msgRaw := strings.TrimSpace(r.URL.Query().Get("validate-input"))

	// Try to parse the structural FIX message
	msg, err := message.MessageFromStringAuto(msgRaw)
	if err != nil {
		w.Header().Set("HX-Trigger", "toolbox-validation-failed")
		obs := []string{fmt.Sprintf("Structural Error: Invalid fix string input - %s", err.Error())}
		renderTemplate(app.templ, w, "partials/toolbox/validate/report", map[string]any{"Observations": obs})
		return
	}

	// Collect MsgId, MsgName from input
	var msgType, msgName string
	if f, pos := msg.FindFrom(35, 0); pos != -1 {
		msgType = f.Value
		ro := app.Session().Router()
		sp := ro.SpecForMsgType(msgType)
		msgName = sp.Messages[msgType].Name
	}

	// Validation strictness determined by the config parameter
	vmode := spec.ValidationBasic
	if app.Store.Config().FixValidateStrict {
		vmode = spec.ValidationStrict
	}

	// Spec Dictionary Validation
	result, _ := app.Session().Router().Validate(&msg, vmode)
	if len(result) > 0 {
		w.Header().Set("HX-Trigger", "toolbox-validation-failed")
	} else {
		w.Header().Set("HX-Trigger", "toolbox-validation-passed")
	}

	renderTemplate(app.templ, w, "partials/toolbox/validate/report", map[string]any{
		"Observations": result, "MsgType": msgType, "MsgName": msgName,
	})
}
