package gui

import (
	"fmt"
	"net/http"

	"github.com/infinage/microfix/cmd/mxgui/internal/models"
	"github.com/infinage/microfix/cmd/mxgui/internal/views"
)

func (app *Application) handleHome(w http.ResponseWriter, r *http.Request) {
	snap := app.Session.Status()
	cfg := app.Store.Config()

	dashData := &models.DashboardData{Snap: snap, Config: cfg}
	modalData := &models.ModalData{IpAddr: cfg.IpAddr, Port: cfg.Port}
	sendMsgData := &models.SendMessageData{Router: app.Session.Router(), Aliases: &cfg.Alias}

	views.HomePage(dashData, modalData, sendMsgData).Render(app.Ctx, w)
}

func (app *Application) handleAPIConnect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK) // Return 200 so HTMX renders the error toast
		views.Toast("error", "Failed to parse form").Render(app.Ctx, w)
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
		views.Toast("error", fmt.Sprintf("Connection Failed: %v", err)).Render(app.Ctx, w)
	} else {
		// Trigger state refresh AND tell Alpine to close the modal
		w.Header().Set("HX-Trigger", "session-updated, close-modal")
		views.Toast("success", fmt.Sprintf("Started %s on %s", mode, addr)).Render(app.Ctx, w)
	}
}

func (app *Application) handleAPIDisconnect(w http.ResponseWriter, r *http.Request) {
	app.Session.Close()
	w.Header().Set("HX-Trigger", "session-updated")
	views.Toast("success", "Session disconnected").Render(app.Ctx, w)
}

func (app *Application) handleAPIReset(w http.ResponseWriter, r *http.Request) {
	app.Session.Close()
	sess, err := NewSession(app.Store.Config())
	if err != nil {
		views.Toast("error", fmt.Sprintf("Failed to reset session: %v", err)).Render(app.Ctx, w)
	}

	app.Session = sess
	w.Header().Set("HX-Trigger", "session-updated")
	views.Toast("success", "Session reset successfully").Render(app.Ctx, w)
}

func (app *Application) handleAPIHeader(w http.ResponseWriter, r *http.Request) {
	snap := app.Session.Status()
	cfg := app.Store.Config()
	views.Header(&models.DashboardData{Snap: snap, Config: cfg}).Render(app.Ctx, w)
}

func (app *Application) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from your ringbuf.CircularBuffer here.
	// For now, returning dummy data to prove the polling works.
	// In production, you might pass an offset/cursor query param so you only fetch NEW logs.
	dummyLogs := []string{
		"8=FIX.4.4|9=65|35=A|34=142|49=SERVER|56=CLIENT|10=114|",
	}
	views.LogEntries(dummyLogs).Render(app.Ctx, w)
}
