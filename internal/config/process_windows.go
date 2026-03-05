//go:build windows

package config

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// isProcessAlive checks whether a process with the given PID exists.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windows.STILL_ACTIVE
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
