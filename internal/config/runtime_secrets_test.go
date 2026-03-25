package config

import (
	"errors"
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestInstanceTokenRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := WriteInstanceToken(InstanceServer, os.Getpid(), "secret-token"); err != nil {
		t.Fatalf("WriteInstanceToken() error = %v", err)
	}

	got, err := ReadInstanceToken(InstanceServer, os.Getpid())
	if err != nil {
		t.Fatalf("ReadInstanceToken() error = %v", err)
	}
	if got != "secret-token" {
		t.Fatalf("ReadInstanceToken() = %q, want %q", got, "secret-token")
	}
}

func TestRemoveInstanceToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := WriteInstanceToken(InstanceServer, os.Getpid(), "secret-token"); err != nil {
		t.Fatalf("WriteInstanceToken() error = %v", err)
	}
	if err := RemoveInstanceToken(InstanceServer, os.Getpid()); err != nil {
		t.Fatalf("RemoveInstanceToken() error = %v", err)
	}
	_, err := ReadInstanceToken(InstanceServer, os.Getpid())
	if !errors.Is(err, ErrRuntimeSecretNotFound) {
		t.Fatalf("ReadInstanceToken() error = %v, want %v", err, ErrRuntimeSecretNotFound)
	}
}

func TestBrowserLaunchSingleUse(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	ticket, err := CreateBrowserLaunch(BrowserLaunchPayload{
		Path:  "/lite/",
		Token: "secret-token",
		Fragment: url.Values{
			"session_path": []string{"session.jsonl"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBrowserLaunch() error = %v", err)
	}

	payload, err := ConsumeBrowserLaunch(ticket)
	if err != nil {
		t.Fatalf("ConsumeBrowserLaunch() error = %v", err)
	}
	if payload.Path != "/lite/" {
		t.Fatalf("payload.Path = %q, want %q", payload.Path, "/lite/")
	}
	if payload.Token != "secret-token" {
		t.Fatalf("payload.Token = %q, want %q", payload.Token, "secret-token")
	}
	if got := payload.Fragment.Get("session_path"); got != "session.jsonl" {
		t.Fatalf("payload.Fragment[session_path] = %q, want %q", got, "session.jsonl")
	}

	_, err = ConsumeBrowserLaunch(ticket)
	if !errors.Is(err, ErrRuntimeSecretNotFound) {
		t.Fatalf("second ConsumeBrowserLaunch() error = %v, want %v", err, ErrRuntimeSecretNotFound)
	}
}

func TestBrowserLaunchExpiry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	ticket, err := CreateBrowserLaunch(BrowserLaunchPayload{
		Token:     "secret-token",
		ExpiresAt: time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatalf("CreateBrowserLaunch() error = %v", err)
	}

	_, err = ConsumeBrowserLaunch(ticket)
	if !errors.Is(err, ErrRuntimeSecretExpired) {
		t.Fatalf("ConsumeBrowserLaunch() error = %v, want %v", err, ErrRuntimeSecretExpired)
	}
}

func TestRuntimeSecretPermissionsUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := WriteInstanceToken(InstanceServer, os.Getpid(), "secret-token"); err != nil {
		t.Fatalf("WriteInstanceToken() error = %v", err)
	}

	dirPath, err := runtimeDir()
	if err != nil {
		t.Fatalf("runtimeDir() error = %v", err)
	}
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", dirPath, err)
	}
	if got := dirInfo.Mode().Perm(); got != DirPerms {
		t.Fatalf("runtime dir perms = %04o, want %04o", got, DirPerms)
	}

	tokenPath, err := instanceTokenPath(InstanceServer, os.Getpid())
	if err != nil {
		t.Fatalf("instanceTokenPath() error = %v", err)
	}
	fileInfo, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", tokenPath, err)
	}
	if got := fileInfo.Mode().Perm(); got != runtimeFilePerms {
		t.Fatalf("token file perms = %04o, want %04o", got, runtimeFilePerms)
	}
}
