package server

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed webapp/*
var webappFS embed.FS

// staticHandler returns an http.Handler that serves the embedded webapp files
// with the API URL injected into index.html.
func staticHandler(port int) http.Handler {
	// Get the webapp subdirectory
	sub, err := fs.Sub(webappFS, "webapp")
	if err != nil {
		// This should never happen with a valid embed
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve index.html with injected API URL for root or index.html requests
		path := r.URL.Path
		if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
			// SPA: serve index.html for all non-file routes
			serveIndexWithAPIURL(w, sub, port)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// serveIndexWithAPIURL reads index.html and injects the API URL meta tag.
func serveIndexWithAPIURL(w http.ResponseWriter, fsys fs.FS, port int) {
	html, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}

	// Inject the actual API port the server is running on
	apiURL := fmt.Sprintf("http://localhost:%d", port)
	metaTag := fmt.Sprintf(`<meta name="thinkt-api-url" content="%s">`, apiURL)
	modifiedHTML := bytes.Replace(html, []byte("<head>"), []byte("<head>\n  "+metaTag), 1)

	w.Header().Set("Content-Type", "text/html")
	w.Write(modifiedHTML)
}
