package gui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/infinage/microfix/pkg/executor"
	"github.com/infinage/microfix/pkg/session"
)

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
