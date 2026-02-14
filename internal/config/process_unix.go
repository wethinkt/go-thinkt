//go:build !windows

package config

import "syscall"

// isProcessAlive checks whether a process with the given PID exists.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// signal 0 tests for process existence without actually sending a signal
	err := syscall.Kill(pid, 0)
	return err == nil
}
