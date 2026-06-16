package gui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/infinage/microfix/cmd/mxgui/internal/diff"
	"github.com/infinage/microfix/cmd/mxgui/internal/inspector"
	"github.com/infinage/microfix/pkg/executor"
	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
)

func renderTemplate(templ *template.Template, w io.Writer, templateName string, data any) {
	err := templ.ExecuteTemplate(w, templateName, data)
	if err != nil {
		fmt.Printf("Failed to render '%s': %s\n", templateName, err.Error())
	}
}

func toast(w http.ResponseWriter, templ *template.Template, typeStr, msg string) {
	w.Header().Set("HX-Reswap", "none")
	renderTemplate(templ, w, "partials/global/toast", map[string]string{"type": typeStr, "message": msg})
}

func (app *Application) handleHome(w http.ResponseWriter, r *http.Request) {
	snap := app.Session.Status()
	cfg := app.Store.Config()

	renderTemplate(app.templ, w, "index.html", map[string]any{
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
	renderTemplate(app.templ, w, "partials/global/header", map[string]any{
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
			renderTemplate(app.templ, &buf, "partials/stream/logs/entry", log)
			logMarkup := strings.ReplaceAll(buf.String(), "\n", " ")
			fmt.Fprintf(w, "data: %s\n\n", logMarkup)
			flusher.Flush()
		}
	}
}

func (app *Application) handleAPIGetAlias(w http.ResponseWriter, r *http.Request) {
	aliasName := r.URL.Query().Get("alias")
	if alias, ok, _ := app.Store.Get("ALIAS." + aliasName); ok {
		w.Write([]byte(alias))
	} else {
		toast(w, app.templ, "error", fmt.Sprintf("Alias not found: %s", aliasName))
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

func (app *Application) handleAPIDictionaryMessage(w http.ResponseWriter, r *http.Request) {
	msgId := r.PathValue("id")

	router := app.Session.Router()

	sp := router.SpecForMsgType(msgId)
	msgEntry, ok := sp.Messages[msgId]
	if !ok {
		toast(w, app.templ, "error", fmt.Sprintf("MsgId [%v] not found", msgId))
		return
	}

	cfg := app.Store.Config()
	sampleMsg, err := router.Sample(msgId, spec.SampleOptions{IncludeOptional: cfg.FixSampleOptional})

	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("MsgId [%v] not found", msgId))
		return
	}

	var flattenedMsgSpec []FieldInfo
	if err = flattenMessageSpec(&flattenedMsgSpec, msgEntry, sp, cfg.SpecDisplayOptFields); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Unexpected error, please try again: %s", err.Error()))
		return
	}

	msgDetail := map[string]any{
		"Id":        msgId,
		"Name":      msgEntry.Name,
		"IsAdmin":   router.IsAdmin(msgId),
		"SampleStr": sampleMsg.String("|"),
		"Entries":   flattenedMsgSpec,
	}

	renderTemplate(app.templ, w, "partials/dictionary/message", msgDetail)
}

func (app *Application) handleAPIDictionaryField(w http.ResponseWriter, r *http.Request) {
	var tag uint16

	tagStr := r.PathValue("tag")
	if tagInt, err := strconv.Atoi(tagStr); err != nil {
		toast(w, app.templ, "error", "Tag is not a valid integer")
		return
	} else {
		tag = uint16(tagInt)
	}

	fieldDef, ok := app.Session.Router().Field(tag)
	if !ok {
		toast(w, app.templ, "error", fmt.Sprintf("Tag [%d] not found", tag))
		return
	}

	sessMsgs, appMsgs := app.Session.Router().SessionSpec().Messages,
		app.Session.Router().ApplSpec().Messages

	// For now we only do a surface level lookup - map to prevent dups
	var usedIn = make(map[string]string)
	for _, sp := range []map[string]spec.Entry{sessMsgs, appMsgs} {
		for msgId, msgSpec := range sp {
			if _, ok := msgSpec.Lookup[tag]; ok {
				usedIn[msgId] = msgSpec.Name
			}
		}
	}

	dictFieldDetail := map[string]any{"FieldDef": fieldDef, "UsedIn": usedIn}
	renderTemplate(app.templ, w, "partials/dictionary/field", dictFieldDetail)
}

func (app *Application) handleAPIAliasNameCheck(w http.ResponseWriter, r *http.Request) {
	aliasName := r.URL.Query().Get("aliasName")
	if aliasName == "" {
		fmt.Fprint(w, `<span id="alias-check" class="text-[10px] text-gray-500 mt-1">Enter an alias name</span>`)
		return
	}

	if _, ok, _ := app.Store.Get("ALIAS." + aliasName); ok {
		fmt.Fprint(w, `<span id="alias-check" class="text-[10px] text-red-400 mt-1">Alias already exists</span>`)
		return
	}

	fmt.Fprint(w, `<span id="alias-check" class="text-[10px] text-green-400 mt-1">Alias available</span>`)
}

func (app *Application) handleAPIAddAlias(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	aliasName := r.FormValue("aliasName")
	aliasValue := r.FormValue("aliasValue")
	if aliasName == "" || aliasValue == "" {
		toast(w, app.templ, "error", "Alias name / template cannot be empty")
		return
	}

	app.Store.Set("ALIAS."+aliasName, aliasValue)
	app.SaveConfig()
	w.Header().Set("HX-Trigger", "close-modal, refresh-alias")
	toast(w, app.templ, "success", "Alias saved")
}

func (app *Application) handleAPIListAlias(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("from") == "settings" {
		renderTemplate(app.templ, w, "partials/settings/alias", map[string]any{"Aliases": app.Store.Config().Alias})
	} else {
		renderTemplate(app.templ, w, "partials/stream/send_form/aliases", map[string]any{"Aliases": app.Store.Config().Alias})
	}
}

func (app *Application) handleAPIDeleteAlias(w http.ResponseWriter, r *http.Request) {
	aliasName := r.PathValue("aliasName")
	_, ok, _ := app.Store.Unset("ALIAS." + aliasName)
	if !ok {
		toast(w, app.templ, "error", "Alias not found!")
		return
	}

	app.SaveConfig()
	w.Header().Set("HX-Trigger", "refresh-alias")
	toast(w, app.templ, "success", "Alias deleted!")
}

func (app *Application) handleAPISaveConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	for _, fieldName := range []string{
		"IpAddr",
		"Port",
		"SenderCompID",
		"TargetCompID",
		"HeartbeatInt",
		"DefaultTimeoutSec",
		"SessionSpec",
		"ApplicationSpec",
		"SpecDisplayOptFields",
		"FixValidateStrict",
		"FixSampleOptional",
		"SkipLatencyCheckInValidate",
	} {
		val := r.FormValue(fieldName)
		if _, _, err := app.Store.Set("CFG."+fieldName, val); err != nil {
			errMsg := fmt.Sprintf("Failed to update [%s]: [%s]", fieldName, err.Error())
			toast(w, app.templ, "error", errMsg)
			return
		}
	}

	if !app.SaveConfig() {
		toast(w, app.templ, "error", "Config save failed")
		return
	}

	toast(w, app.templ, "success", "Config save successful")
}

func (app *Application) handleAPILoadConfig(w http.ResponseWriter, r *http.Request) {
	_, _, err := r.FormFile("configFile")
	if err == nil {
		// TODO: File uploaded, cannot get the full filepath
		toast(w, app.templ, "error", "File upload not supported at the moment")
		return
	} else {
		// No file provided, 'discard' action
		if err = app.Store.LoadConfig(app.Store.ConfigPath()); err != nil {
			toast(w, app.templ, "error", "Failed to reload from disk")
			return
		}

		// Notify reload successful
		renderTemplate(app.templ, w, "partials/global/toast", map[string]string{"type": "success", "message": "Reload successful"})
	}

	renderTemplate(app.templ, w, "partials/settings/config/form", map[string]any{"partials/settings/config": app.Store.Config()})
}

func (app *Application) handleAPIDumpConfig(w http.ResponseWriter, r *http.Request) {
	data, err := json.MarshalIndent(app.Store.Config(), "", "  ")
	if err != nil {
		toast(w, app.templ, "error", "Failed to dump config")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"config.mxrc\"")
	w.Write(data)
}

func (app *Application) handleAPIConfigSpecPathCheck(w http.ResponseWriter, r *http.Request) {
	specPath := ""
	if r.URL.Query().Get("from") == "session-spec" {
		specPath = r.URL.Query().Get("SessionSpec")
	} else {
		specPath = r.URL.Query().Get("ApplicationSpec")
	}

	// Clear icon if empty
	if specPath == "" {
		w.Write([]byte(""))
		return
	}

	// Embedded dictionary paths
	validSpecs := map[string]any{
		"FIX40": nil, "FIX41": nil, "FIX42": nil, "FIX43": nil, "FIX44": nil,
		"FIX50": nil, "FIX50SP1": nil, "FIX50SP2": nil, "FIXT11": nil,
	}

	var checkResults = make(map[string]string)
	_, err := os.Stat(specPath)
	_, ok1 := validSpecs[specPath]
	_, ok2 := validSpecs[strings.TrimSuffix(specPath, ".xml")]
	if ok1 || ok2 || err == nil {
		checkResults["Color"] = "green"
		checkResults["PathData"] = "M5 13l4 4L19 7"
		checkResults["Text"] = "Valid dictionary found"
	} else {
		checkResults["Color"] = "red"
		checkResults["PathData"] = "M6 18L18 6M6 6l12 12"
		checkResults["Text"] = "File not found or invalid path"
	}

	renderTemplate(app.templ, w, "partials/settings/config/spec_path_check", checkResults)
}

func (app *Application) handleAPIInspect(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("message")
	_, err := message.MessageFromString(raw, "|")
	if err != nil {
		toast(w, app.templ, "error", "Failed to parse message")
		return
	}

	vmode := spec.ValidationStrict
	if !app.Store.Config().FixValidateStrict {
		vmode = spec.ValidationBasic
	}

	w.Header().Set("HX-Trigger", "open-inspector-tab")
	inspectViewData := inspector.NewInspectView(raw, app.Session.Router(), vmode)
	renderTemplate(app.templ, w, "partials/stream/inspector", inspectViewData)
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
	diffs := diff.Compare(source, target)
	for idx := range diffs {
		diffs[idx].Name = "Unknown"
		if fdef, ok := app.Session.Router().Field(diffs[idx].Tag); ok {
			diffs[idx].Name = fdef.Name
		}
	}
	renderTemplate(app.templ, w, "partials/stream/inspector/diff_output", diffs)
}

func (app *Application) handleAPIScriptUpload(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("scriptFile")
	if err != nil {
		w.Write([]byte(`<div sse-swap="done" @htmx:load="alert('Upload failed')"></div>`))
		return
	}
	defer file.Close()

	verbose := r.FormValue("verbose") == "true"

	// Create temp file
	tempFile, err := os.CreateTemp("", "microfix-script-*.mxs")
	if err != nil {
		w.Write([]byte(`<div sse-swap="done" @htmx:load="alert('Temp file creation failed')"></div>`))
		return
	}
	defer tempFile.Close()
	io.Copy(tempFile, file)

	html := fmt.Sprintf(`
		<div hx-ext="sse" sse-connect="/api/script/stream?file=%s&verbose=%t">
			<div sse-swap="log" hx-target="#script-console" hx-swap="beforeend"></div>
			<div sse-swap="done" hx-target="#sse-injector" hx-swap="innerHTML" @htmx:sse-message="running = false">
            </div>
		</div>
	`, tempFile.Name(), verbose)

	w.Write([]byte(html))
}

func (app *Application) handleAPIScriptStream(w http.ResponseWriter, r *http.Request) {
	// Singal that output is an event-stream
	w.Header().Set("Content-Type", "text/event-stream")

	// Read filepath
	fpath := r.URL.Query().Get("file")
	verbose := r.URL.Query().Get("verbose") == "true"
	defer os.Remove(fpath) // Cleanup when done

	// Add a small buffer
	sseChan := make(chan string, 20)

	// Initialize context
	writer := sseWriter{stream: sseChan}
	scriptCtx, stop := executor.NewScriptContext(app.Session, app.Store, &writer)
	defer stop() // Failsafe cleanup

	var wg sync.WaitGroup

	// Verbose Logs (Goroutine 1)
	if verbose {
		if logCh, unsub, err := app.Session.SubscribeLog(); err == nil {
			defer unsub()
			wg.Go(func() {
				router := app.Session.Router()

				for {
					select {
					case <-scriptCtx.GoCtx.Done(): // Triggers when script finishes
						return
					case log, ok := <-logCh:
						if !ok {
							return
						}
						colorClass := "text-gray-500"
						hint := ""

						switch log.Type {
						case session.LogSys:
							colorClass = "text-yellow-400"
						case session.LogErr:
							colorClass = "text-red-400"
						case session.LogSend, session.LogRecv:
							if log.Type == session.LogSend {
								colorClass = "text-blue-400"
							} else {
								colorClass = "text-green-400"
							}
							if msgType, ok := log.Msg.Get(35); ok {
								if entry, ok := router.SpecForMsgType(msgType).Messages[msgType]; ok {
									hint = entry.Name
								}
							}
						}

						html := fmt.Sprintf(`<div class="%s break-all">%s</div>`, colorClass, log.String(hint))
						sseChan <- html
					}
				}
			})
		}
	}

	// Script Runner (Goroutine 2)
	wg.Go(func() {
		defer stop() // Cancels scriptCtx, which tells the Logger to exit

		// Read temp file created at `/api/script/upload`
		f, err := os.Open(fpath)
		if err != nil {
			sseChan <- fmt.Sprintf(`<div class="text-red-500">Failed to read file: %v</div>`, err)
			return
		}
		defer f.Close()

		if err := executor.EvalBatch(f, &scriptCtx); err != nil {
			sseChan <- fmt.Sprintf(`<div class="text-red-400">✗ Script Failed: %v</div>`, err)
		} else {
			sseChan <- `<div class="text-green-400">✓ Script Completed.</div>`
		}
	})

	// Orchestrator (Goroutine 3)
	// Simply waits for Runner and Logger to finish, then safely closes the channel
	go func() {
		wg.Wait()
		close(sseChan)
	}()

	// Main Event Loop
	flusher, _ := w.(http.Flusher)
	for htmlContent := range sseChan {
		fmt.Fprintf(w, "event: log\ndata: %s\n\n", htmlContent)
		flusher.Flush()
	}

	// Safely send termination signal ONLY after sseChan is completely drained
	fmt.Fprintf(w, "event: done\ndata: \n\n")
	flusher.Flush()
}
