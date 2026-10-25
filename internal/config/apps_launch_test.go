package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"nimbleterminal.dev/ntopenwith"
)

func TestLaunchPreservesPathArgument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project with spaces; $HOME")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		argv []string
		want []string
	}{
		{"editor", []string{"code", "{}"}, []string{"code", path}},
		{"terminal", []string{"ghostty", "--working-directory={}"}, []string{"ghostty", "--working-directory=" + path}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := AppConfig{ID: tt.name, Exec: tt.argv}
			launchErr := errors.New("launcher failed")
			err := app.launch(path, nil, func(argv []string) error {
				if !reflect.DeepEqual(argv, tt.want) {
					t.Errorf("argv = %q, want %q", argv, tt.want)
				}
				return launchErr
			})
			if !errors.Is(err, launchErr) {
				t.Fatalf("launch error = %v, want %v", err, launchErr)
			}
		})
	}
}

func TestLaunchRejectsMissingPath(t *testing.T) {
	app := AppConfig{ID: "editor", Exec: []string{"code", "{}"}}
	err := app.launch(filepath.Join(t.TempDir(), "missing"), nil, func([]string) error {
		t.Fatal("must not start an app for a missing path")
		return nil
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("launch error = %v, want ErrNotExist", err)
	}
}

func TestLaunchSystemFileManager(t *testing.T) {
	id := map[string]string{"darwin": "finder", "windows": "explorer", "linux": "files", "freebsd": "files"}[runtime.GOOS]
	if id == "" {
		t.Skip("no system file manager on this platform")
	}
	for _, directory := range []bool{false, true} {
		path := t.TempDir()
		if !directory {
			path = filepath.Join(path, "session.jsonl")
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		launchErr := errors.New("system launcher failed")
		called := false
		source := launchSource{launch: func(target ntopenwith.Target, verb ntopenwith.Verb, app ntopenwith.App) error {
			called = true
			if target.Path != path || target.Directory != directory || verb != ntopenwith.Open || app.ID != ntopenwith.SystemID {
				t.Errorf("unexpected system launch: %+v, %q, %+v", target, verb, app)
			}
			return launchErr
		}}
		app := AppConfig{ID: id}
		err := app.launch(path, source, func([]string) error {
			t.Fatal("system file manager must use the OS source")
			return nil
		})
		if !called || !errors.Is(err, launchErr) {
			t.Fatalf("called = %v, error = %v; want system launch error", called, err)
		}
	}
}

type launchSource struct {
	launch func(ntopenwith.Target, ntopenwith.Verb, ntopenwith.App) error
}

func (s launchSource) Apps(context.Context, ntopenwith.Target, ntopenwith.Verb) ([]ntopenwith.App, error) {
	return nil, nil
}

func (s launchSource) Launch(_ context.Context, target ntopenwith.Target, verb ntopenwith.Verb, app ntopenwith.App) (*exec.Cmd, error) {
	return nil, s.launch(target, verb, app)
}
