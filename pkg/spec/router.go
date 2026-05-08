package spec

import (
	"fmt"
	"strings"
)

type Router struct {
	sessSpec    *Spec
	applSpecs   map[string]*Spec
	defaultAppl string
}

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
		sessSpec:  &sessSpec,
		applSpecs: make(map[string]*Spec),
	}

	for _, path := range applSpecPaths {
		spec, err := LoadSpec(path)
		if err != nil {
			return nil, fmt.Errorf("Failed to load Appl spec %s: %v", path, err)
		}

		// First spec is set as default spec
		specID := spec.BeginString()
		router.applSpecs[specID] = &spec
		if router.defaultAppl == "" {
			router.defaultAppl = specID
		}
	}

	return router, nil
}

func NewDefaultRouter(sessSpecPath string) (*Router, error) {
	// FIXT - Load all appl specs
	if strings.HasPrefix(sessSpecPath, "FIXT") {
		applSpecPaths := []string{"FIX40.xml", "FIX41.xml", "FIX42.xml", "FIX43.xml", "FIX44.xml", "FIX50.xml"}
		router, err := NewRouter(sessSpecPath, applSpecPaths)
		if err != nil {
			return nil, err
		}

		router.SwitchApplSpec("FIX.5.0")
		return router, nil
	}

	// Legacy session and appl spec are the same
	return NewRouter(sessSpecPath, []string{sessSpecPath})
}

// Switch between laoded application level specs
func (router *Router) SwitchApplSpec(specID string) bool {
	if _, ok := router.applSpecs[specID]; !ok {
		return false
	}

	router.defaultAppl = specID
	return true
}

// List Appl layer specs available
func (router *Router) ListApplSpecs() []string {
	var specs []string
	for name := range router.applSpecs {
		specs = append(specs, name)
	}
	return specs
}

// Get default Appl spec selected
func (router *Router) DefaultApplSpec() *Spec {
	return router.applSpecs[router.defaultAppl]
}

// For Symetery to DefaultApplSpec
func (router *Router) DefaultSessionSpec() *Spec {
	return router.sessSpec
}
