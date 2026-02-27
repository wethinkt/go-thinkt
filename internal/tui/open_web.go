package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

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

	baseURL := fmt.Sprintf("http://%s:%d", inst.Host, inst.Port)

	// Build hash fragment with deep-link params and auth token
	params := url.Values{}
	if inst.Token != "" {
		params.Set("token", inst.Token)
	}
	if sessionPath != "" {
		params.Set("session_path", sessionPath)
	}
	if projectPath != "" {
		params.Set("project_id", projectPath)
	}

	openURL := baseURL
	if len(params) > 0 {
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
