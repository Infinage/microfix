package gui

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"

	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type Application struct {
	Session *session.Session
	Store   *store.Store
	Ctx     context.Context

	port   int
	assets embed.FS
	templ  *template.Template

	isWailsApp bool // conditional rendering of templates
	wails      *application.App
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
		"getSpecName":            getSpecName,
		"getThemeForEngineState": getThemeForEngineState,
		"getThemeForLogType":     getThemeForLogType,
		"toTitle":                toTitle,
		"getAllFieldNamesAsJSON": getAllFieldNamesAsJSON,
		"replaceSOH":             replaceSOH,
	}

	var err error
	templ := template.New("").Funcs(templHelpers)

	templ, err = templ.ParseFS(assets, "assets/html/pages/*html", "assets/html/partials/*/*html")
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

// Returns true if config exists and save successful
func (app *Application) SaveConfig() bool {
	st := app.Store
	savePath := st.ConfigPath()
	if _, err := os.Stat(savePath); err == nil {
		if err = st.DumpConfig(savePath); err == nil {
			return true
		}
	}
	return false
}

func (app *Application) StartWails() error {
	// Config wails with middleware to intercept all requests
	mux := app.routes()

	app.isWailsApp = true
	app.wails = application.New(application.Options{
		Name:        "MicroFix",
		Description: "High-performance FIX Protocol client",
		Assets: application.AssetOptions{
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/wails/about/repository":
						app.handleWailsAboutRepository(w, r)
					case "/wails/about/contact":
						app.handleWailsAboutMailto(w, r)
					case "/wails/config/import":
						app.handleWailsImportConfig(w, r)
					default:
						mux.ServeHTTP(w, r)
					}
				})
			},
		},
	})

	// Start a new window
	app.wails.Window.New()

	// Cleanup on WailsApp exit
	app.wails.OnShutdown(func() {
		app.SaveConfig()
	})

	// Blocks until UI closes
	if err := app.wails.Run(); err != nil {
		return err
	}

	return nil
}

// addr: ":0" can be passed to use a randomized port listening on localhost.
// Alternatively use ":3000" or similar combined with 'air' when building UI
//
// Deprecated: Merely for prototyping and development.
func (app *Application) StartWeb(addr string) error {
	defer app.SaveConfig()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	defer listener.Close()
	app.port = listener.Addr().(*net.TCPAddr).Port
	fmt.Println("Listening on port:", app.port)

	if err = http.Serve(listener, app.routes()); err != nil {
		return err
	}

	fmt.Println("Closing application")
	return nil
}

func (app *Application) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.FileServerFS(app.assets))

	mux.HandleFunc("GET /{$}", app.handleHome)

	mux.HandleFunc("GET /api/header", app.handleAPIHeader)
	mux.HandleFunc("POST /api/connect", app.handleAPIConnect)
	mux.HandleFunc("GET /api/reset", app.handleAPIReset)
	mux.HandleFunc("GET /api/disconnect", app.handleAPIDisconnect)
	mux.HandleFunc("GET /api/logs/stream", app.handleAPILogs)
	mux.HandleFunc("GET /api/sample", app.handleAPISample)
	mux.HandleFunc("POST /api/send", app.handleAPISend)
	mux.HandleFunc("GET /api/finalize", app.handleAPIFinalize)
	mux.HandleFunc("GET /api/validate", app.handleAPIValidate)

	mux.HandleFunc("GET /api/dictionary/message/{id}", app.handleAPIDictionaryMessage)
	mux.HandleFunc("GET /api/dictionary/field/{tag}", app.handleAPIDictionaryField)

	mux.HandleFunc("GET /api/alias/get", app.handleAPIGetAlias)
	mux.HandleFunc("GET /api/alias/list", app.handleAPIListAlias)
	mux.HandleFunc("DELETE /api/alias/delete/{aliasName}", app.handleAPIDeleteAlias)
	mux.HandleFunc("POST /api/alias/add", app.handleAPIAddAlias)
	mux.HandleFunc("GET /api/alias/check/name", app.handleAPIAliasNameCheck)

	mux.HandleFunc("POST /api/config", app.handleAPISaveConfig)
	mux.HandleFunc("POST /api/config/reset", app.handleAPIResetConfig)
	mux.HandleFunc("GET /api/config/export", app.handleAPIDumpConfig)
	mux.HandleFunc("GET /api/config/check/specpath", app.handleAPIConfigSpecPathCheck)

	mux.HandleFunc("GET /api/inspect", app.handleAPIInspect)
	mux.HandleFunc("POST /api/diff", app.handleAPIMessageDiff)
	mux.HandleFunc("POST /api/script/upload", app.handleAPIScriptUpload)
	mux.HandleFunc("GET /api/script/stream", app.handleAPIScriptStream)

	return mux
}
