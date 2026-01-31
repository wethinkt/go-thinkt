package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed webapp/*
var webappFS embed.FS

// staticHandler returns an http.Handler that serves the embedded webapp files.
func staticHandler() http.Handler {
	// Get the webapp subdirectory
	sub, err := fs.Sub(webappFS, "webapp")
	if err != nil {
		// This should never happen with a valid embed
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
