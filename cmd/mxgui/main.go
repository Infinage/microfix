package main

import (
	"embed"
	gui "github.com/infinage/microfix/cmd/mxgui/internal/handlers"
)

//go:embed assets/*
var assets embed.FS

func main() {
	app := gui.NewApplication(assets)
	app.Start()
}
