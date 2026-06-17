package gui

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

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
			renderTemplate(app.templ, &buf, "partials/stream/logs/entry", log)
			logMarkup := strings.ReplaceAll(buf.String(), "\n", " ")
			fmt.Fprintf(w, "data: %s\n\n", logMarkup)
			flusher.Flush()
		}
	}
}

func (app *Application) handleAPISend(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || len(r.FormValue("message")) < 4 {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	msgRaw := r.FormValue("message")
	raw := r.FormValue("raw") == "yes"

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

func (app *Application) handleAPISample(w http.ResponseWriter, r *http.Request) {
	cfg := app.Store.Config()
	msgType := r.URL.Query().Get("msgtype")
	msg, err := app.Session.Router().Sample(msgType, spec.SampleOptions{IncludeOptional: cfg.FixSampleOptional})
	if err == nil {
		w.Write([]byte(msg.String("|")))
	} else {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to sample: %s", err.Error()))
	}
}
