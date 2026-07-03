package gui

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/infinage/microfix/pkg/executor"
	"github.com/infinage/microfix/pkg/session"
)

func (app *Application) handleAPIScriptExecute(w http.ResponseWriter, r *http.Request) {
	scriptContent := r.FormValue("scriptContent")
	verbose := r.FormValue("verbose") == "true"

	// Create temp file
	tempFile, err := os.CreateTemp("", "microfix-script-*.mxs")
	if err != nil {
		w.Header().Set("HX-Trigger", "script-error")
		toast(w, app.templ, "error", "Temp file creation failed, please try again")
		return
	}

	_, err = tempFile.WriteString(scriptContent)
	tempFile.Close()
	if err != nil {
		w.Header().Set("HX-Trigger", "script-error")
		toast(w, app.templ, "error", "Failed to write script")
		return
	}

	// If using wails route through sse server
	sseURL := ""
	if app.isWailsApp {
		sseURL = fmt.Sprintf("http://localhost:%d", app.port)
	}

	html := fmt.Sprintf(`
		<div hx-ext="sse" sse-connect="%s/api/script/stream?file=%s&verbose=%t">
			<div sse-swap="log" hx-target="#script-console" hx-swap="beforeend"></div>
			<div sse-swap="done" hx-target="#sse-injector" hx-swap="innerHTML" @htmx:sse-message="running = false">
            </div>
		</div>
	`, sseURL, url.QueryEscape(tempFile.Name()), verbose)

	w.Write([]byte(html))
}

func (app *Application) handleAPIScriptCancel(w http.ResponseWriter, _ *http.Request) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.cancelScript != nil {
		app.cancelScript()
		toast(w, app.templ, "success", "Script cancelled")
	}
}

func (app *Application) handleAPIScriptStream(w http.ResponseWriter, r *http.Request) {
	// Check if flushing is supported
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: log\ndata: <div class=\"text-red-500 font-bold mb-2\">Error: HTTP flush not supported by the server/proxy.</div>\n\n")
		fmt.Fprintf(w, "event: done\ndata: \n\n")
		return
	}

	// Singal that output is an event-stream
	w.Header().Set("Content-Type", "text/event-stream")

	// Read filepath
	fpath := r.URL.Query().Get("file")
	verbose := r.URL.Query().Get("verbose") == "true"
	defer os.Remove(fpath) // Cleanup when done

	// Reject in case this API is invoked externally
	clean := filepath.Clean(fpath)
	if !strings.HasPrefix(clean, filepath.Clean(os.TempDir())) ||
		!strings.HasPrefix(filepath.Base(clean), "microfix-script-") {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Add a small buffer
	sseChan := make(chan string, 20)

	// Initialize context
	writer := sseWriter{stream: sseChan}
	scriptCtx, stop := executor.NewScriptContext(app.Session, app.resetSession, app.Store, &writer)
	defer stop() // Failsafe cleanup

	// Store the cancelfunc, callable from another API
	app.mu.Lock()
	app.cancelScript = stop
	app.mu.Unlock()

	var wg sync.WaitGroup

	// Verbose Logs (Goroutine 1)
	if verbose {
		logCh, unsub := app.logBroker.Subscribe()
		wg.Go(func() {
			defer unsub()
			router := app.Session().Router()
			logSanitizer := strings.NewReplacer("<", "", ">", "")

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
					case session.LogInfo:
						colorClass = "text-yellow-400"
					case session.LogTran:
						colorClass = "text-orange-400"
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

					logStr := logSanitizer.Replace(log.String(hint))
					html := fmt.Sprintf(`<div class="%s break-all">%s</div>`, colorClass, logStr)
					sseChan <- html
				}
			}
		})
	}

	// Script Runner (Goroutine 2)
	wg.Go(func() {
		defer stop() // Cancels scriptCtx, which tells the Logger to exit (Goroutine 1)

		// Read temp file created via api `/api/script/execute`
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
Loop:
	for {
		select {
		case <-r.Context().Done():
			return
		case htmlContent, ok := <-sseChan:
			if !ok {
				break Loop
			}
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", htmlContent)
			flusher.Flush()
		}
	}

	// Safely send termination signal ONLY after sseChan is completely drained
	fmt.Fprintf(w, "event: done\ndata: \n\n")
	flusher.Flush()
}
