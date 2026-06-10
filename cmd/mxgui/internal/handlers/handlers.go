package gui

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

func toast(w http.ResponseWriter, templ *template.Template, typeStr, msg string) {
	w.Header().Set("HX-Reswap", "none")
	templ.ExecuteTemplate(w, "Toast", map[string]string{"type": typeStr, "message": msg})
}

func (app *Application) handleHome(w http.ResponseWriter, r *http.Request) {
	snap := app.Session.Status()
	cfg := app.Store.Config()

	app.templ.ExecuteTemplate(w, "index.html", map[string]any{
		"Snapshot": snap,
		"Config":   cfg,
		"Router":   app.Session.Router(),
		"Aliases":  &cfg.Alias,
	})
}

func (app *Application) handleAPIConnect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	host, port, mode := r.FormValue("host"), r.FormValue("port"), r.FormValue("mode")
	addr := fmt.Sprintf("%s:%s", host, port)

	var err error
	if mode == "client" {
		err = app.Session.Connect(addr)
	} else {
		err = app.Session.Listen(addr)
	}

	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Connection Failed: %v", err.Error()))
	} else {
		w.Header().Set("HX-Trigger", "session-updated, close-modal")
		toast(w, app.templ, "success", fmt.Sprintf("Started %s on %s", mode, addr))
	}
}

func (app *Application) handleAPIDisconnect(w http.ResponseWriter, r *http.Request) {
	app.Session.Close()
	w.Header().Set("HX-Trigger", "session-updated")
	toast(w, app.templ, "success", "Session disconnected")
}

func (app *Application) handleAPIReset(w http.ResponseWriter, r *http.Request) {
	app.Session.Close()
	sess, err := NewSession(app.Store.Config())
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to reset session: %v", err))
		return
	}

	app.Session = sess
	w.Header().Set("HX-Trigger", "session-updated")
	toast(w, app.templ, "success", "Session reset successfully")
}

func (app *Application) handleAPIHeader(w http.ResponseWriter, r *http.Request) {
	app.templ.ExecuteTemplate(w, "Header", map[string]any{
		"Snapshot": app.Session.Status(),
		"Config":   app.Store.Config(),
	})
}

func (app *Application) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	// Set header for Server sent events (SSE)
	w.Header().Set("Content-Type", "text/event-stream")

	flusher, ok := w.(http.Flusher)
	if !ok {
		toast(w, app.templ, "error", "Streaming unsupported")
		return
	}

	// Subscribe to logs from session
	logCh, closeLogs, err := app.Session.SubscribeLog()
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to subscribe log: %s", err.Error()))
		return
	}
	defer closeLogs()

	// Continuously poll the logs and push to the server
	for {
		select {
		case <-r.Context().Done():
			return
		case log, ok := <-logCh:
			if !ok {
				return
			}

			// Parse and print the logs
			var buf bytes.Buffer
			app.templ.ExecuteTemplate(&buf, "LogEntry", log)
			logMarkup := strings.ReplaceAll(buf.String(), "\n", " ")
			fmt.Fprintf(w, "data: %s\n\n", logMarkup)
			flusher.Flush()
		}
	}
}

func (app *Application) handleAPIGetAlias(w http.ResponseWriter, r *http.Request) {
	aliasName := r.URL.Query().Get("alias")
	alias, ok, _ := app.Store.Get("ALIAS." + aliasName)
	if ok {
		w.Write([]byte(alias))
	} else {
		toast(w, app.templ, "error", fmt.Sprintf("Alias not found: %s", aliasName))
	}
}

func (app *Application) handleAPISample(w http.ResponseWriter, r *http.Request) {
	msgType := r.URL.Query().Get("msgtype")
	msg, err := app.Session.Router().Sample(msgType, spec.SampleOptions{})
	if err == nil {
		w.Write([]byte(msg.String("|")))
	} else {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to sample: %s", err.Error()))
	}
}

func (app *Application) handleAPISend(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || len(r.FormValue("message")) < 4 {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	msgRaw := r.Form.Get("message")
	raw := r.Form.Get("raw") == "yes"

	delim := msgRaw[len(msgRaw)-1:]
	msg, err := message.MessageFromString(msgRaw, delim)
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to parse input string: %s", err.Error()))
		return
	}

	if err = app.Session.Send(msg, raw); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to send message: %s", err.Error()))
		return
	}
}

func (app *Application) handleAPIFinalize(w http.ResponseWriter, r *http.Request) {
	msgRaw := r.URL.Query().Get("finalize-input")
	if len(msgRaw) < 4 {
		toast(w, app.templ, "error",  "Input must be atleast 4 chars long")
		return
	}

	delim := msgRaw[len(msgRaw)-1:]
	msg, err := message.MessageFromString(msgRaw, delim)
	if err != nil {
		toast(w, app.templ, "error",  fmt.Sprintf("Invalid fix string input: %s", err.Error()))
		return
	}

	// Finalize the message and return fragment
	msg.Finalize()
	w.Write([]byte(msg.String(delim)))
}

func (app *Application) handleAPIValidate(w http.ResponseWriter, r *http.Request) {
	msgRaw := r.URL.Query().Get("validate-input")
	if len(msgRaw) < 4 {
		app.templ.ExecuteTemplate(w, "ValidationReport", []string{"Structural Error: Input must be at least 4 chars long"})
		return
	}

	// Try to parse the structural FIX message
	delim := msgRaw[len(msgRaw)-1:]
	msg, err := message.MessageFromString(msgRaw, delim)
	if err != nil {
		errMsg := fmt.Sprintf("Structural Error: Invalid fix string input - %s", err.Error())
		app.templ.ExecuteTemplate(w, "ValidationReport", []string{errMsg})
		return
	}

	// Spec Dictionary Validation
	result, _ := app.Session.Router().Validate(&msg, spec.ValidationStrict)
	app.templ.ExecuteTemplate(w, "ValidationReport", result)
}

func (app *Application) handleAPIDictionaryMessage(w http.ResponseWriter, r *http.Request) {
	msgId := r.PathValue("id")

	router := app.Session.Router()

	sp := router.SpecForMsgType(msgId)
	msgEntry, ok := sp.Messages[msgId]
	if !ok {
		toast(w, app.templ, "error", fmt.Sprintf("MsgId [%v] not found", msgId))
		return
	}

	sampleMsg, err := router.Sample(msgId, spec.SampleOptions{})
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("MsgId [%v] not found", msgId))
		return
	}

	flattenedMsgSpec, err := flattenMessageSpec(msgEntry, sp)
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Unexpected error, please try again: %s", err.Error()))
		return
	}

	msgDetail := map[string]any{
		"Id": msgId,
		"IsAdmin": router.IsAdmin(msgId),
		"SampleStr": sampleMsg.String("|"),
		"Entries": flattenedMsgSpec, 
	}

	app.templ.ExecuteTemplate(w, "DictionaryMessageDetail", msgDetail)
}

func (app *Application) handleAPIDictionaryField(w http.ResponseWriter, r *http.Request) {
	app.templ.ExecuteTemplate(w, "DictionaryFieldDetail", nil)
}
