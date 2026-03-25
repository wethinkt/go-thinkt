package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wethinkt/go-thinkt/internal/config"
)

func (s *HTTPServer) handleBrowserLaunch(w http.ResponseWriter, r *http.Request) {
	payload, err := config.ConsumeBrowserLaunch(chi.URLParam(r, "ticket"))
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, config.ErrRuntimeSecretExpired) {
			status = http.StatusGone
		}
		http.Error(w, http.StatusText(status), status)
		return
	}

	target := sanitizeBrowserLaunchTarget(payload.Path)
	fragment := url.Values{}
	for key, values := range payload.Fragment {
		for _, value := range values {
			fragment.Add(key, value)
		}
	}
	if payload.Token != "" {
		fragment.Set("token", payload.Token)
	}

	location := target
	if encoded := fragment.Encode(); encoded != "" {
		location += "#" + encoded
	}
	http.Redirect(w, r, location, http.StatusFound)
}

func sanitizeBrowserLaunchTarget(target string) string {
	if target == "" {
		return "/"
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "/"
	}
	if !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") {
		return "/"
	}

	location := parsed.Path
	if parsed.RawQuery != "" {
		location += "?" + parsed.RawQuery
	}
	return location
}
