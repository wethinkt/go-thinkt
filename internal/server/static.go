package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web-lite/index.html
//go:embed web-lite/static/*
var webappFS embed.FS

// staticHandler returns an http.Handler that serves the embedded webapp files.
func staticHandler() http.Handler {
	// Get the webapp subdirectory
	sub, err := fs.Sub(webappFS, "web-lite")
	if err != nil {
		// This should never happen with a valid embed
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
			// SPA: serve index.html for all non-file routes
			data, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
