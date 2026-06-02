package gui

import (
	"embed"
	"net/http"

	"github.com/a-h/templ"
	"github.com/infinage/microfix/cmd/mxgui/internal/views"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

type Application struct {
	Session *session.Session
	Store   *store.Store

	assets embed.FS
}

func NewApplication(assets embed.FS) *Application {
	st := store.InitStore()
	return &Application{Store: &st, assets: assets}
}

func (app *Application) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.FileServerFS(app.assets))
	mux.Handle("GET /", templ.Handler(views.Layout()))
	return mux
}

func (app *Application) Start() error {
	mux := app.routes()
	if err := http.ListenAndServe(":3000", mux); err != nil {
		return err
	}
	return nil
}
