package main

import (
	"embed"
	"fmt"

	gui "github.com/infinage/microfix/cmd/mxgui/internal/handlers"
)

//go:embed assets/*
var assets embed.FS

func main() {
	app, err := gui.NewApplication(assets)
	if err != nil {
		fmt.Printf("Failed to start application: %v\n", err)
	}
	app.Start()
	fmt.Println("Closing application")
}
