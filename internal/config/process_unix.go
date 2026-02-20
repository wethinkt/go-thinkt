//go:build !windows

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
	// signal 0 tests for process existence without actually sending a signal
	err := syscall.Kill(pid, 0)
	return err == nil
}

// stopProcess sends SIGTERM on Unix.
func stopProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}

// applyPlatformBackgroundFlags applies Unix-specific flags for backgrounding.
func applyPlatformBackgroundFlags(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.Setsid = true
}
