package gui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/infinage/microfix/pkg/session"
	"github.com/infinage/microfix/pkg/spec"
	"github.com/infinage/microfix/pkg/store"
)

func (app *Application) handleAPIGetAlias(w http.ResponseWriter, r *http.Request) {
	aliasName := r.URL.Query().Get("alias")
	if alias, ok, _ := app.Store.Get("ALIAS." + aliasName); ok {
		w.Write([]byte(alias))
	} else {
		toast(w, app.templ, "error", fmt.Sprintf("Alias not found: %s", aliasName))
	}
}

func (app *Application) handleAPIAliasNameCheck(w http.ResponseWriter, r *http.Request) {
	aliasName := r.URL.Query().Get("aliasName")
	if aliasName == "" {
		fmt.Fprint(w, `<span id="alias-check" class="text-[10px] text-gray-500 mt-1">Enter an alias name</span>`)
		return
	}

	if _, ok, _ := app.Store.Get("ALIAS." + aliasName); ok {
		fmt.Fprint(w, `<span id="alias-check" class="text-[10px] text-red-400 mt-1">Alias already exists</span>`)
		return
	}

	fmt.Fprint(w, `<span id="alias-check" class="text-[10px] text-green-400 mt-1">Alias available</span>`)
}

func (app *Application) handleAPIAddAlias(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	aliasName := r.FormValue("aliasName")
	aliasValue := r.FormValue("aliasValue")
	if aliasName == "" || aliasValue == "" {
		toast(w, app.templ, "error", "Alias name / template cannot be empty")
		return
	}

	app.Store.Set("ALIAS."+aliasName, aliasValue)
	app.SaveConfig()
	w.Header().Set("HX-Trigger", "close-modal, refresh-alias")
	toast(w, app.templ, "success", "Alias saved")
}

func (app *Application) handleAPIListAlias(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("from") == "settings" {
		renderTemplate(app.templ, w, "partials/settings/aliases", map[string]any{"Aliases": app.Store.Config().Alias})
	} else {
		renderTemplate(app.templ, w, "partials/stream/send_form/select/aliases", map[string]any{"Aliases": app.Store.Config().Alias})
	}
}

func (app *Application) handleAPIDeleteAlias(w http.ResponseWriter, r *http.Request) {
	aliasName := r.PathValue("aliasName")
	_, ok, _ := app.Store.Unset("ALIAS." + aliasName)
	if !ok {
		toast(w, app.templ, "error", "Alias not found!")
		return
	}

	app.SaveConfig()
	w.Header().Set("HX-Trigger", "refresh-alias")
	toast(w, app.templ, "success", "Alias deleted!")
}

func (app *Application) handleAPISaveConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		toast(w, app.templ, "error", "Failed to parse form")
		return
	}

	for _, fieldName := range []string{
		"IpAddr",
		"Port",
		"SenderCompID",
		"TargetCompID",
		"HeartbeatInt",
		"DefaultTimeoutSec",
		"SessionSpec",
		"ApplicationSpec",
		"SpecDisplayOptFields",
		"FixValidateStrict",
		"FixSampleOptional",
		"SkipLatencyCheckInValidate",
	} {
		val := r.FormValue(fieldName)
		if _, _, err := app.Store.Set("CFG."+fieldName, val); err != nil {
			errMsg := fmt.Sprintf("Failed to update [%s]: [%s]", fieldName, err.Error())
			toast(w, app.templ, "error", errMsg)
			return
		}
	}

	if !app.SaveConfig() {
		toast(w, app.templ, "error", "Config save failed")
		return
	}

	// Attempt to reset session with changes if not already started
	toastMsg := "Configuration saved successfully. Changes will be applied after the next session reset."
	if app.Session().Status().State == session.SessionNew {
		if err := app.resetSession(); err != nil {
			toast(w, app.templ, "error", fmt.Sprintf("Config saved, but reset failed: %v", err))
			return
		}

		// Update listening components - header, dictionary, stream select boxes
		w.Header().Set("HX-Trigger", "config-reloaded, session-updated")
		toastMsg = "Configuration saved and applied successfully."
	}

	toast(w, app.templ, "success", toastMsg)
}

func (app *Application) handleAPIResetConfig(w http.ResponseWriter, r *http.Request) {
	if app.Store.LoadConfig(app.Store.ConfigPath()) != nil {
		toast(w, app.templ, "error", "Failed to reload from disk")
		return
	}

	// Notify reload successful - without setting hx-reswap:none
	data := map[string]string{"type": "success", "message": "Reload successful"}
	renderTemplate(app.templ, w, "partials/global/toast", data)

	// Data for rendering config form
	formData := map[string]any{
		"Config":     app.Store.Config(),
		"ConfigPath": app.Store.ConfigPath(),
		"ConfigHelp": store.ConfigHelp,
	}

	// Reload config page
	renderTemplate(app.templ, w, "partials/settings/config/form", formData)
}

func (app *Application) handleAPIDumpConfig(w http.ResponseWriter, r *http.Request) {
	data, err := json.MarshalIndent(app.Store.Config(), "", "  ")
	if err != nil {
		toast(w, app.templ, "error", "Failed to dump config")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"config.mxrc\"")
	w.Write(data)
}

func (app *Application) handleAPIConfigSpecPathCheck(w http.ResponseWriter, r *http.Request) {
	specPath := ""
	if r.URL.Query().Get("from") == "session-spec" {
		specPath = r.URL.Query().Get("SessionSpec")
	} else {
		specPath = r.URL.Query().Get("ApplicationSpec")
	}

	// Clear icon if empty
	if specPath == "" {
		w.Write([]byte(""))
		return
	}

	// Check if path available and return a nice cue
	var checkResults = make(map[string]string)
	if ok := spec.CheckPath(specPath); ok {
		checkResults["Color"] = "green"
		checkResults["PathData"] = "M5 13l4 4L19 7"
		checkResults["Text"] = "Valid dictionary found"
	} else {
		checkResults["Color"] = "red"
		checkResults["PathData"] = "M6 18L18 6M6 6l12 12"
		checkResults["Text"] = "File not found or invalid path"
	}

	renderTemplate(app.templ, w, "partials/settings/config/spec_path_check", checkResults)
}
