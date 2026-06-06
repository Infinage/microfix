package gui

import (
	"fmt"
	"net/http"
)

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
		w.WriteHeader(http.StatusOK)
		app.templ.ExecuteTemplate(w, "Toast", map[string]string{"type": "error", "message": "Failed to parse form"})
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
		app.templ.ExecuteTemplate(w, "Toast", map[string]string{
			"type":    "error",
			"message": fmt.Sprintf("Connection Failed: %v", err),
		})
	} else {
		w.Header().Set("HX-Trigger", "session-updated, close-modal")
		app.templ.ExecuteTemplate(w, "Toast", map[string]string{
			"type":    "success",
			"message": fmt.Sprintf("Started %s on %s", mode, addr),
		})
	}
}

func (app *Application) handleAPIDisconnect(w http.ResponseWriter, r *http.Request) {
	app.Session.Close()
	w.Header().Set("HX-Trigger", "session-updated")
	app.templ.ExecuteTemplate(w, "Toast", map[string]string{
		"type": "success", "message": "Session disconnected",
	})
}

func (app *Application) handleAPIReset(w http.ResponseWriter, r *http.Request) {
	app.Session.Close()
	sess, err := NewSession(app.Store.Config())
	if err != nil {
		app.templ.ExecuteTemplate(w, "Toast", map[string]string{
			"type":    "error",
			"message": fmt.Sprintf("Failed to reset session: %v", err),
		})
		return
	}

	app.Session = sess
	w.Header().Set("HX-Trigger", "session-updated")
	app.templ.ExecuteTemplate(w, "Toast", map[string]string{
		"type": "success", "message": "Session reset successfully",
	})
}

func (app *Application) handleAPIHeader(w http.ResponseWriter, r *http.Request) {
	app.templ.ExecuteTemplate(w, "Header", map[string]any{
		"Snapshot": app.Session.Status(),
		"Config":   app.Store.Config(),
	})
}

func (app *Application) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	dummyLogs := []string{
		"8=FIX.4.4|9=65|35=A|34=142|49=SERVER|56=CLIENT|10=114|",
	}
	app.templ.ExecuteTemplate(w, "LogEntries", dummyLogs)
}
