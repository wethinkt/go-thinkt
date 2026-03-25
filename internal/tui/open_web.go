package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// openInWeb opens the thinkt web interface in the default browser,
// optionally deep-linking to a specific project and/or session.
func openInWeb(projectPath, sessionPath string) {
	inst := config.FindInstanceByType(config.InstanceServer)
	if inst == nil {
		tuilog.Log.Warn("openInWeb: no running server found")
		return
	}

	host := inst.Host
	if host == "" {
		host = "localhost"
	}
	baseURL := fmt.Sprintf("http://%s:%d", host, inst.Port)

	// Build hash fragment with deep-link params. If the server is authenticated,
	// a short-lived launch ticket carries the token outside argv.
	params := url.Values{}
	if sessionPath != "" {
		params.Set("session_path", sessionPath)
	}
	if projectPath != "" {
		params.Set("project_id", projectPath)
	}

	openURL := baseURL + "/"
	token, err := config.ReadInstanceToken(config.InstanceServer, inst.PID)
	if err == nil && token != "" {
		ticket, launchErr := config.CreateBrowserLaunch(config.BrowserLaunchPayload{
			Path:     "/",
			Token:    token,
			Fragment: params,
		})
		if launchErr == nil {
			openURL = strings.TrimRight(baseURL, "/") + "/launch/" + ticket
		}
	}
	if !strings.Contains(openURL, "/launch/") && len(params) > 0 {
		openURL += "#" + params.Encode()
	}

	tuilog.Log.Info("openInWeb", "url", baseURL, "project", projectPath, "session", sessionPath)
	openBrowser(openURL)
}

// openBrowser opens a URL in the default browser.
func openBrowser(rawURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return
	}
	_ = cmd.Start()
}
