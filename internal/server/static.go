package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web-lite/index.html
//go:embed web-lite/assets/*
var webappLiteFS embed.FS

//go:embed web/index.html
//go:embed web/assets/*.js
//go:embed web/assets/styles/*.css
var webappFS embed.FS

// StaticLiteWebAppHandler returns an http.Handler that serves the embedded web-lite app.
func StaticLiteWebAppHandler() http.Handler {
	sub, err := fs.Sub(webappLiteFS, "web-lite")
	if err != nil {
		panic(err) // This should never happen with a valid embed
	}
	return spaHandler(sub)
}

// StaticWebAppHandler returns an http.Handler that serves the full thinkt-web app.
func StaticWebAppHandler() http.Handler {
	sub, err := fs.Sub(webappFS, "web")
	if err != nil {
		panic(err) // This should never happen with a valid embed
	}
	return spaHandler(sub)
}

// spaHandler returns an http.Handler that serves static files from fsys,
// falling back to index.html for non-file routes (SPA routing).
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
			data, err := fs.ReadFile(fsys, "index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
