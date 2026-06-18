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
		fmt.Printf("Failed to init application: %v\n", err)
		os.Exit(1)
	}

	//app.StartWeb(":3000") // ":0" for randomized port
	app.StartWails()

	fmt.Println("Closing application")
}
