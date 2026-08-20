package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/migrate"
	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/store"
)

func (app *Application) handleWailsAboutRepository(http.ResponseWriter, *http.Request) {
	app.wails.Browser.OpenURL("https://github.com/infinage/microfix")
}

func (app *Application) handleWailsAboutDocs(http.ResponseWriter, *http.Request) {
	app.wails.Browser.OpenURL("https://github.com/Infinage/microfix/blob/main/README.md")
}

func (app *Application) handleWailsAboutMailto(http.ResponseWriter, *http.Request) {
	app.wails.Browser.OpenURL("mailto:nj.deesa@gmail.com")
}

func (app *Application) handleWailsImportConfig(w http.ResponseWriter, _ *http.Request) {
	// OpenFile Dialog from wails runtime
	dialog := app.wails.Dialog.OpenFile()
	dialog.SetTitle("Load MicroFIX Configuration")
	dialog.AddFilter("MicroFIX Config", "*.mxrc")
	dialog.AddFilter("All Files", "*.*")

	// Show the dialog. This blocks until the user selects a file or cancels.
	fpath, err := dialog.PromptForSingleSelection()
	if err != nil || fpath == "" {
		toast(w, app.templ, "error", "Failed to select file")
		return
	}

	// Load config from file, original config untouched on error
	if err := app.Store.LoadConfig(fpath); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to load config: %s", err.Error()))
		return
	}

	// If session not started, apply changes
	toastData := map[string]string{"type": "success", "message": "Config imported successfully, changes will be applied after session reset."}
	if app.Session().Status().State == session.SessionNew {
		if err := app.resetSession(); err != nil {
			toastData["message"] = "Config imported, but reset failed"
			renderTemplate(app.templ, w, "partials/global/toast", toastData)
			return
		}

		// Update listening components - header, dictionary, stream select boxes
		w.Header().Set("HX-Trigger", "config-reloaded, refresh-alias, session-updated")
		toastData["message"] = "Configuration imported and applied successfully."
	}

	// Data for rendering config form
	formData := map[string]any{
		"Config":     app.Store.Config(),
		"ConfigPath": app.Store.ConfigPath(),
		"ConfigHelp": store.ConfigHelp,
	}

	// Reload config page
	renderTemplate(app.templ, w, "partials/global/toast", toastData)
	renderTemplate(app.templ, w, "partials/settings/config/form", formData)
}

func (app *Application) handleWailsExportConfig(w http.ResponseWriter, _ *http.Request) {
	dialog := app.wails.Dialog.SaveFile()
	dialog.AddFilter("MicroFIX config", "*.mxrc")
	dialog.AddFilter("All Files", "*.*")

	fpath, err := dialog.PromptForSingleSelection()
	if err != nil || fpath == "" {
		toast(w, app.templ, "error", "Failed to select path")
		return
	}

	if err = app.Store.DumpConfig(fpath); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to dump config: %s", err.Error()))
		return
	}

	toast(w, app.templ, "success", fmt.Sprintf("Config saved to '%s'", fpath))
}

func (app *Application) handleWailsExportLogs(w http.ResponseWriter, _ *http.Request) {
	dialog := app.wails.Dialog.SaveFile()
	dialog.AddFilter("MicroFIX log", "*.log")

	fpath, err := dialog.PromptForSingleSelection()
	if err != nil || fpath == "" {
		toast(w, app.templ, "error", "Failed to select path")
		return
	}

	if err := app.tlogger.Dump(fpath); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to dump log: %s", err.Error()))
		return
	}

	toast(w, app.templ, "success", fmt.Sprintf("Logs written to '%s'", fpath))
}

func (app *Application) handleWailsImportAlias(w http.ResponseWriter, _ *http.Request) {
	// OpenFile Dialog from wails runtime
	dialog := app.wails.Dialog.OpenFile()
	dialog.SetTitle("Import Alias")
	dialog.AddFilter("MicroFIX Config", "*.mxrc")
	dialog.AddFilter("MiniFIX Config", "*.xml")

	// Show the dialog. This blocks until the user selects a file or cancels.
	fpath, err := dialog.PromptForSingleSelection()
	if err != nil || fpath == "" {
		toast(w, app.templ, "error", "Failed to select file")
		return
	}

	var parsedAliases []map[string]any
	var failedAliases []string
	var formatDetected string

	switch ext := strings.ToLower(path.Ext(fpath)); ext {
	case ".mxrc":
		var st *store.Store
		formatDetected = "MicroFIX (.mxrc)"
		if st, err = store.NewStoreFromPath(fpath); err == nil {
			for name, templateStr := range st.Config().Alias {
				parsedAliases = append(parsedAliases, map[string]any{
					"name":     name,
					"template": templateStr,
					"selected": true,
				})
			}
		}

	case ".xml":
		var file *os.File
		formatDetected = "MiniFIX (.xml)"
		aliasMap := make(map[string]message.Message)
		if file, err = os.Open(fpath); err == nil {
			defer file.Close()
			if aliasMap, failedAliases, err = migrate.ExtractAliasFromMiniFIX(file); err == nil {
				ro := app.Session().Router()
				for name, msg := range aliasMap {
					msg = ro.Salvage(msg)
					parsedAliases = append(parsedAliases, map[string]any{
						"name":     name,
						"template": msg.String("\x01"),
						"selected": true,
					})
				}
			}
		}

	default: // Wails will disallow selecting files with unknown extension already
		err = fmt.Errorf("unsupported config format: %s", ext)
	}

	if err == nil && len(parsedAliases) == 0 {
		err = fmt.Errorf("no aliases found - is this a valid %s file?", formatDetected)
	}

	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to load config: %s", err.Error()))
		return
	}

	// Marshal data to JSON strings for safe frontend injection
	aliasesJSON, _ := json.Marshal(parsedAliases)
	if string(aliasesJSON) == "null" {
		aliasesJSON = []byte("[]")
	}
	failedJSON, _ := json.Marshal(failedAliases)
	if string(failedJSON) == "null" {
		failedJSON = []byte("[]")
	}

	renderTemplate(app.templ, w, "partials/modals/import_alias", map[string]any{
		"FormatDetected": formatDetected,
		"ParsedAliases":  string(aliasesJSON),
		"FailedAliases":  string(failedJSON),
	})
}
