package gui

import (
	"fmt"
	"net/http"
)

func (app *Application) handleWailsAboutRepository(http.ResponseWriter, *http.Request) {
	app.wails.Browser.OpenURL("https://github.com/infinage/microfix")
}

func (app *Application) handleWailsAboutMailto(http.ResponseWriter, *http.Request) {
	app.wails.Browser.OpenURL("mailto:nj.deesa@gmail.com")
}

func (app *Application) handleWailsImportConfig(w http.ResponseWriter, _ *http.Request) {
	// OpenFile Dialog from wails runtime
	dialog := app.wails.Dialog.OpenFile()
	dialog.SetTitle("Load MicroFix Configuration")
	dialog.AddFilter("MicroFix Config", "*.mxrc")
	dialog.AddFilter("All Files", "*.*")
	dialog.CanChooseDirectories(true)

	// Show the dialog. This blocks until the user selects a file or cancels.
	filePath, err := dialog.PromptForSingleSelection()
	if err != nil {
		toast(w, app.templ, "error", "Failed to open dialog")
		return
	}

	// Load config from file, original config untouched on error
	if err := app.Store.LoadConfig(filePath); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Failed to load config: %s", err.Error()))
		return
	}

	// Reload the config page
	renderTemplate(app.templ, w, "partials/settings/config/form",
		map[string]any{"partials/settings/config": app.Store.Config()})
}
