package gui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/infinage/microfix/cmd/mxgui/internal/diff"
	"github.com/infinage/microfix/cmd/mxgui/internal/inspector"
	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

func (app *Application) handleAPIInspect(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("message")
	if len(raw) < 4 {
		toast(w, app.templ, "error", "Input must be atleast 4 chars long")
		return
	}

	delim := raw[len(raw)-1:]
	_, err := message.MessageFromString(raw, delim)
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to parse message: %s", err.Error()))
		return
	}

	vmode := spec.ValidationStrict
	if !app.Store.Config().FixValidateStrict {
		vmode = spec.ValidationBasic
	}

	w.Header().Set("HX-Trigger", "open-inspector-tab")
	logType := r.URL.Query().Get("LogType")
	inspectViewData := inspector.NewInspectView(raw, logType, app.Session().Router(), vmode)
	renderTemplate(app.templ, w, "partials/stream/inspector/layout", inspectViewData)
}

func (app *Application) handleAPIMessageDiff(w http.ResponseWriter, r *http.Request) {
	sourceStr, targetStr := r.FormValue("source"), r.FormValue("target")

	// Empty input
	targetStr = strings.TrimSpace(targetStr)
	if targetStr == "" {
		renderTemplate(app.templ, w, "partials/stream/inspector/diff_empty", nil)
		return
	}

	// Invalid input
	if len(sourceStr) < 4 || len(targetStr) < 4 {
		renderTemplate(app.templ, w, "partials/stream/inspector/diff_malformed", map[string]string{"Error": "Input must be at least 4 chars long"})
		return
	}
	source, err := message.MessageFromString(sourceStr, sourceStr[len(sourceStr)-1:])
	if err != nil {
		renderTemplate(app.templ, w, "partials/stream/inspector/diff_malformed", map[string]string{"Error": fmt.Sprintf("Malformed source string: %s", err.Error())})
		return
	}
	target, err := message.MessageFromString(targetStr, targetStr[len(targetStr)-1:])
	if err != nil {
		renderTemplate(app.templ, w, "partials/stream/inspector/diff_malformed", map[string]string{"Error": fmt.Sprintf("Malformed target string: %s", err.Error())})
		return
	}

	// Render diffs
	sess := app.Session()
	diffs := diff.Compare(source, target)
	for idx := range diffs {
		diffs[idx].Name = "Unknown"
		if fdef, ok := sess.Router().Field(diffs[idx].Tag); ok {
			diffs[idx].Name = fdef.Name
		}
	}
	renderTemplate(app.templ, w, "partials/stream/inspector/diff_output", diffs)
}
