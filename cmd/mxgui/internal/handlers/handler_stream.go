package gui

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/pretty"
	"github.com/infinage/microfix/pkg/spec"
)

var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
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

			// Parse and append logs to temp file logger
			buf1 := bufferPool.Get().(*bytes.Buffer)
			buf1.Reset()
			pretty.Log(buf1, log, app.Session.Router())
			app.tlogger.Log(buf1.String())
			bufferPool.Put(buf1)

			// Parse and print the logs for GUI
			buf2 := bufferPool.Get().(*bytes.Buffer)
			buf2.Reset()
			renderTemplate(app.templ, buf2, "partials/stream/logs/entry", log)
			logMarkup := strings.ReplaceAll(buf2.String(), "\n", " ")
			bufferPool.Put(buf2)
			fmt.Fprintf(w, "data: %s\n\n", logMarkup)
			flusher.Flush()
		}
	}
}

func (app *Application) handleAPIExportLogs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Disposition", "attachment; filename=microfix_stream_export.log")
	w.Header().Set("Content-Type", "text/plain")

	if err := app.tlogger.DumpTo(w); err != nil {
		http.Error(w, "Failed to export logs", http.StatusInternalServerError)
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
