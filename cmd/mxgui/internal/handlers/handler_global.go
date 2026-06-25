package gui

import (
	"fmt"
	"net/http"
	"strconv"
)

func (app *Application) handleAPIConnect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	host, port, mode := r.FormValue("host"), r.FormValue("port"), r.FormValue("mode")
	addr := fmt.Sprintf("%s:%s", host, port)

	var err error
	if sess := app.Session(); mode == "client" {
		err = sess.Connect(addr)
	} else {
		err = sess.Listen(addr)
	}

	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Connection Failed: %v", err.Error()))
	} else {
		w.Header().Set("HX-Trigger", "session-updated, close-modal")
		toast(w, app.templ, "success", fmt.Sprintf("Started %s on %s", mode, addr))
	}
}

func (app *Application) handleAPIDisconnect(w http.ResponseWriter, r *http.Request) {
	app.Session().Close()
	w.Header().Set("HX-Trigger", "session-updated")
	toast(w, app.templ, "success", "Session disconnected")
}

func (app *Application) handleAPIReset(w http.ResponseWriter, r *http.Request) {
	if err := app.resetSession(); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Reset failed: %v", err))
		return
	}

	// Trigger updation of headers and other listening components
	w.Header().Set("HX-Trigger", "config-reloaded, session-updated")
	toast(w, app.templ, "success", "Session reset successfully")
}

func (app *Application) handleAPIResetSequence(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	inSeqStr := r.FormValue("inSeq")
	inSeq, err := strconv.ParseInt(inSeqStr, 10, 64)
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Not a valid Seq no [InSeq]: %s", inSeqStr))
		return
	}

	outSeqStr := r.FormValue("outSeq")
	outSeq, err := strconv.ParseInt(outSeqStr, 10, 64)
	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Not a valid Seq no [OutSeq]: %s", outSeqStr))
		return
	}

	if err = app.Session().ResetSequence(inSeq, outSeq); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to reset sequence: %s", err.Error()))
		return
	}

	w.Header().Set("HX-Trigger", "session-updated, close-modal")
	toast(w, app.templ, "success", "Sequence reset successfully")
}

func (app *Application) handleAPIHeader(w http.ResponseWriter, r *http.Request) {
	renderTemplate(app.templ, w, "partials/global/header", map[string]any{
		"Snapshot": app.Session().Status(),
		"Config":   app.Store.Config(),
	})
}
