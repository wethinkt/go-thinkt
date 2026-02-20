//go:build windows

package config

import (
	"os"
	"os/exec"
	"syscall"
)

// isProcessAlive checks whether a process with the given PID exists.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds. Signal(0) is not supported,
	// so we rely on the fact that FindProcess doesn't error for valid PIDs.
	// This is a best-effort check.
	_ = p
	return true
}

// stopProcess on Windows just kills.
func stopProcess(proc *os.Process) error {
	return proc.Kill()
}

// applyPlatformBackgroundFlags applies Windows-specific flags for backgrounding.
func applyPlatformBackgroundFlags(c *exec.Cmd) {
	const CREATE_NEW_PROCESS_GROUP = 0x00000200
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CreationFlags |= CREATE_NEW_PROCESS_GROUP
}
