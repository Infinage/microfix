package gui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/infinage/microfix/pkg/spec"
)

func (app *Application) handleAPIDictionaryDefinitions(w http.ResponseWriter, r *http.Request) {
	renderTemplate(app.templ, w, "partials/dictionary/definitions", map[string]any{"Router": app.Session.Router()})
}

func (app *Application) handleAPIDictionaryMessage(w http.ResponseWriter, r *http.Request) {
	msgId := r.PathValue("id")

	router := app.Session.Router()

	sp := router.SpecForMsgType(msgId)
	msgEntry, ok := sp.Messages[msgId]
	if !ok {
		toast(w, app.templ, "error", fmt.Sprintf("MsgId [%v] not found", msgId))
		return
	}

	cfg := app.Store.Config()
	sampleMsg, err := router.Sample(msgId, spec.SampleOptions{IncludeOptional: cfg.FixSampleOptional})

	if err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("MsgId [%v] not found", msgId))
		return
	}

	var flattenedMsgSpec []FieldInfo
	if err = flattenMessageSpec(&flattenedMsgSpec, msgEntry, sp, cfg.SpecDisplayOptFields); err != nil {
		toast(w, app.templ, "error", fmt.Sprintf("Unexpected error, please try again: %s", err.Error()))
		return
	}

	msgDetail := map[string]any{
		"Id":        msgId,
		"Name":      msgEntry.Name,
		"IsAdmin":   router.IsAdmin(msgId),
		"SampleStr": sampleMsg.String("|"),
		"Entries":   flattenedMsgSpec,
	}

	renderTemplate(app.templ, w, "partials/dictionary/message", msgDetail)
}

func (app *Application) handleAPIDictionaryField(w http.ResponseWriter, r *http.Request) {
	var tag uint16

	tagStr := r.PathValue("tag")
	if tagInt, err := strconv.Atoi(tagStr); err != nil {
		toast(w, app.templ, "error", "Tag is not a valid integer")
		return
	} else {
		tag = uint16(tagInt)
	}

	fieldDef, ok := app.Session.Router().Field(tag)
	if !ok {
		toast(w, app.templ, "error", fmt.Sprintf("Tag [%d] not found", tag))
		return
	}

	sessMsgs, appMsgs := app.Session.Router().SessionSpec().Messages,
		app.Session.Router().ApplSpec().Messages

	// For now we only do a surface level lookup - map to prevent dups
	var usedIn = make(map[string]string)
	for _, sp := range []map[string]spec.Entry{sessMsgs, appMsgs} {
		for msgId, msgSpec := range sp {
			if _, ok := msgSpec.Lookup[tag]; ok {
				usedIn[msgId] = msgSpec.Name
			}
		}
	}

	dictFieldDetail := map[string]any{"FieldDef": fieldDef, "UsedIn": usedIn}
	renderTemplate(app.templ, w, "partials/dictionary/field", dictFieldDetail)
}
