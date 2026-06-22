package gui

import (
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/infinage/microfix/cmd/mxgui/internal/shortcuts"
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
		"AppVersion": app.Version,
		"GitCommit":  app.Commit,
		"Snapshot":   snap,
		"Config":     cfg,
		"Router":     app.Session.Router(),
		"Aliases":    &cfg.Alias,
		"IsWailsApp": app.isWailsApp,
		"Shortcuts":  shortcuts.Shortcuts(),
	})
}
