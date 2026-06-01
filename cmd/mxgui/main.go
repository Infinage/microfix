package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"

	"github.com/a-h/templ"
	"github.com/infinage/microfix/cmd/mxgui/internal/views"
)

//go:embed assets/*
var assets embed.FS

func main() {
	mux := http.NewServeMux()	
	mux.Handle("GET /assets/", http.FileServerFS(assets))
	mux.Handle("GET /", templ.Handler(views.Layout()))

	if err := http.ListenAndServe(":3000", mux); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
}
