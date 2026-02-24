package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

// redactingLogFormatter wraps chi's default formatter and redacts sensitive query params.
type redactingLogFormatter struct {
	base middleware.LogFormatter
}

func (f *redactingLogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return f.base.NewLogEntry(redactRequestForLogging(r))
}

func redactRequestForLogging(r *http.Request) *http.Request {
	if r == nil || r.URL == nil || r.URL.RawQuery == "" {
		return r
	}

	query := r.URL.Query()
	changed := false
	for key := range query {
		if isSensitiveQueryKey(key) {
			query.Set(key, "[REDACTED]")
			changed = true
		}
	}
	if !changed {
		return r
	}

	cloned := r.Clone(r.Context())
	cloned.URL.RawQuery = query.Encode()
	cloned.RequestURI = cloned.URL.RequestURI()
	return cloned
}

func sanitizeQueryForRedirect(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	for key := range values {
		if isSensitiveQueryKey(key) {
			values.Del(key)
		}
	}
	return values.Encode()
}

func isSensitiveQueryKey(key string) bool {
	switch strings.ToLower(key) {
	case "token", "access_token", "refresh_token", "id_token", "authorization", "auth", "api_key", "apikey":
		return true
	default:
		return false
	}
}
