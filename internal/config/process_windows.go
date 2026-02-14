//go:build windows

package config

import "os"

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
