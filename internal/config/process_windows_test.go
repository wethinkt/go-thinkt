//go:build windows

package config

import (
	"io"
	"os/exec"
	"testing"
	"time"
)

func TestIsProcessAlive(t *testing.T) {
	if isProcessAlive(0) {
		t.Fatal("expected pid 0 to be reported as dead")
	}

	cmd := exec.Command("ping", "-n", "2", "127.0.0.1")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start test process: %v", err)
	}

	pid := cmd.Process.Pid
	if !isProcessAlive(pid) {
		_ = cmd.Process.Kill()
		t.Fatalf("expected pid %d to be alive while process is running", pid)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait for test process: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("expected pid %d to be reported as dead after process exit", pid)
}
