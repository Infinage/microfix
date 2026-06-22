package gui

import (
	"fmt"
	"net/http"
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

	// Reload the config page
	renderTemplate(app.templ, w, "partials/settings/config/form",
		map[string]any{"partials/settings/config": app.Store.Config()})
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
