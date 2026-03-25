package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestHandleBrowserLaunchRedirectsWithFragment(t *testing.T) {
	ticket, err := config.CreateBrowserLaunch(config.BrowserLaunchPayload{
		Path:  "/lite/",
		Token: "secret-token",
		Fragment: url.Values{
			"project_id": []string{"demo"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBrowserLaunch() error = %v", err)
	}

	s := NewHTTPServer(thinkt.NewRegistry(), DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/launch/"+ticket, nil)
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusFound)
	}
	if got := rr.Header().Get("Location"); got != "/lite/#project_id=demo&token=secret-token" {
		t.Fatalf("Location = %q, want %q", got, "/lite/#project_id=demo&token=secret-token")
	}
}

func TestHandleBrowserLaunchMissingTicket(t *testing.T) {
	s := NewHTTPServer(thinkt.NewRegistry(), DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/launch/does-not-exist", nil)
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestSanitizeBrowserLaunchTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "empty", target: "", want: "/"},
		{name: "relative", target: "lite/", want: "/"},
		{name: "scheme relative", target: "//evil.example", want: "/"},
		{name: "absolute", target: "https://evil.example", want: "/"},
		{name: "valid", target: "/lite/?view=debug", want: "/lite/?view=debug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeBrowserLaunchTarget(tt.target); got != tt.want {
				t.Fatalf("sanitizeBrowserLaunchTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
