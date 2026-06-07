package main

import (
	"embed"
	"fmt"
	"os"

	gui "github.com/infinage/microfix/cmd/mxgui/internal/handlers"
)

//go:embed assets/*
var assets embed.FS

func main() {
	app, err := gui.NewApplication(assets)
	if err != nil {
		fmt.Printf("Failed to start application: %v\n", err)
		os.Exit(1)
	}
	app.Start()
	fmt.Println("Closing application")
}
