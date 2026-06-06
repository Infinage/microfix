package gui

import (
	"context"
	"embed"
	"html/template"
	"net/http"

	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

type Application struct {
	Session *session.Session
	Store   *store.Store
	Ctx     context.Context

	assets embed.FS
	templ  *template.Template
}

func NewSession(cfg store.Config) (*session.Session, error) {
	return session.NewSession(
		cfg.SessionSpec,
		cfg.SenderCompID,
		cfg.TargetCompID,
		cfg.HeartbeatInt,
		session.EngineOptions{
			DefaultApplVer:   cfg.ApplicationSpec,
			SkipLatencyCheck: cfg.SkipLatencyCheckInValidate,
		})
}

func NewApplication(assets embed.FS) (*Application, error) {
	// Helper functions for parsing templates
	templHelpers := template.FuncMap{
		"getSpecName":  getSpecName,
		"getColorName": getColorName,
	}

	templ, err := template.New("").Funcs(templHelpers).ParseFS(assets, "assets/html/*html")
	if err != nil {
		return nil, err
	}

	st := store.InitStore()
	sess, err := NewSession(st.Config())
	if err != nil {
		return nil, err
	}

	return &Application{
		Session: sess,
		Store:   &st,
		Ctx:     context.Background(),
		assets:  assets,
		templ:   templ,
	}, nil
}

func (app *Application) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.FileServerFS(app.assets))
	mux.HandleFunc("GET /{$}", app.handleHome)
	mux.HandleFunc("GET /api/header", app.handleAPIHeader)
	mux.HandleFunc("POST /api/connect", app.handleAPIConnect)
	mux.HandleFunc("GET /api/reset", app.handleAPIReset)
	mux.HandleFunc("GET /api/disconnect", app.handleAPIDisconnect)
	mux.HandleFunc("GET /api/logs", app.handleAPILogs)
	return mux
}

func (app *Application) Start() error {
	mux := app.routes()
	if err := http.ListenAndServe(":3000", mux); err != nil {
		return err
	}
	return nil
}
