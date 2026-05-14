package spec

import (
	"fmt"
	"path"
	"strings"
)

type Router struct {
	sessSpec       *Spec
	applSpecs      map[string]*Spec // Keyed by Spec ID (e.g., "FIX50")
	defaultApplVer string           // e.g., "FIX50"

	// Maps the wire value of Tag 1128/1137
	// to a Spec ID (e.g., "9" -> "FIX50SP2")
	applVerIDMap map[string]string
}

// Picks up session spec's Tag 1128 [ApplVerID] to determine the enums to route
func NewRouter(sessionSpecPath string, applSpecPaths []string) (*Router, error) {
	sessSpec, err := LoadSpec(sessionSpecPath)
	if err != nil {
		return nil, err
	}

	if err := sessSpec.CheckSessionCapabilities(); err != nil {
		return nil, fmt.Errorf("Session Spec [%s] invalid: %w", sessionSpecPath, err)
	}

	if len(applSpecPaths) == 0 {
		return nil, fmt.Errorf("Expected atleast one Application Spec")
	}

	router := &Router{
		sessSpec:     &sessSpec,
		applSpecs:    make(map[string]*Spec),
		applVerIDMap: make(map[string]string),
	}

	// Store the mapping to translate ApplVerID / DefaultApplVerID
	if applVerID, err := sessSpec.Field(1128); err == nil {
		for _, enum := range applVerID.Enums {
			router.applVerIDMap[enum.Enum] = enum.Description
		}
	}

	// Load the appl specs
	for _, fpath := range applSpecPaths {
		spec, err := LoadSpec(fpath)
		if err != nil {
			return nil, fmt.Errorf("Failed to load Appl spec %s: %v", fpath, err)
		}

		// First spec is set as default spec
		specID := strings.TrimSuffix(path.Base(fpath), ".xml")
		router.applSpecs[specID] = &spec
		if router.defaultApplVer == "" {
			router.defaultApplVer = specID
		}
	}

	return router, nil
}

// Convenience function that auto loads all XMLs if FIXT.xml is set
// Otherwise loads just the specified spec XML.
func NewDefaultRouter(sessSpecPath string) (*Router, error) {
	// FIXT - Load all appl specs
	if strings.HasPrefix(sessSpecPath, "FIXT") {
		applSpecPaths := []string{"FIX40.xml", "FIX41.xml", "FIX42.xml", "FIX43.xml",
			"FIX44.xml", "FIX50.xml", "FIX50SP1.xml", "FIX50SP2.xml"}
		router, err := NewRouter(sessSpecPath, applSpecPaths)
		if err != nil {
			return nil, err
		}

		router.SetDefaultApplVer("FIX.5.0")
		return router, nil
	}

	// Legacy session and appl spec are the same
	return NewRouter(sessSpecPath, []string{sessSpecPath})
}

// Returns currently selected defaultApplVer (FIX wire protocol value)
func (router *Router) GetDefaultApplVerID() string {
	for applVerID, applVerStr := range router.applVerIDMap {
		if applVerStr == router.defaultApplVer {
			return applVerID
		}
	}
	return ""
}

// SetDefaultApplVerID sets the fallback dictionary using the FIX wire protocol value (Tag 1137).
// Example: SetDefaultApplVerID("9") -> Sets to FIX50SP2
func (router *Router) SetDefaultApplVerID(applVerID string) bool {
	specID, ok := router.applVerIDMap[applVerID]
	if !ok {
		return false
	}
	return router.SetDefaultApplVer(specID)
}

// Explicitly sets the application dictionary by its internal name
// eg: SetDefaultApplVerStr("FIX50")
func (router *Router) SetDefaultApplVer(specID string) bool {
	if _, ok := router.applSpecs[specID]; !ok {
		return false
	}
	router.defaultApplVer = specID
	return true
}

// Return default ApplSpec currently selected from 'SetDefaultApplSpec'
func (router *Router) ApplSpec() *Spec {
	return router.applSpecs[router.defaultApplVer]
}

// For Symetery to DefaultApplSpec
func (router *Router) SessionSpec() *Spec {
	return router.sessSpec
}

func (r *Router) IsAdmin(msgType string) bool {
	switch msgType {
	case "0", "1", "2", "3", "4", "5", "A", "n":
		return true
	}
	return false
}

// For AdminMessage, returns SessionSpec otherwise returns ApplSpec
func (r *Router) SpecForMsgType(msgType string) *Spec {
	if r.IsAdmin(msgType) {
		return r.SessionSpec()
	}
	return r.ApplSpec()
}

// Checks if field is available in SessionSpec, if not checks in ApplSpec
func (r *Router) Field(tag uint16) (FieldDef, bool) {
	if fDef, ok := r.SessionSpec().Fields[tag]; ok {
		return fDef, true
	}

	if fDef, ok := r.ApplSpec().Fields[tag]; ok {
		return fDef, true
	}

	return FieldDef{}, false
}
