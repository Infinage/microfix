package main

import (
	"embed"
	"fmt"
	"os"

	gui "github.com/infinage/microfix/cmd/mxgui/internal/handlers"
)

//go:embed assets/*
var assets embed.FS

// Populated from ldflags via GitHub CI/CD
var (
	Version   = "v0.0.0-dev"
	GitCommit = "local"
)

func main() {
	app, err := gui.NewApplication(Version, GitCommit, assets)
	if err != nil {
		fmt.Printf("Failed to init application: %v\n", err)
		os.Exit(1)
	}

	if err = app.StartWails(); err != nil {
		fmt.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}
}
