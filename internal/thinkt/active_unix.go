//go:build !windows

package thinkt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// detectProcess finds active sessions by looking for known AI tool processes
// and mapping their working directories to projects/sessions.
// Uses ps + lsof (available on macOS and Linux).
func (d *ActiveSessionDetector) detectProcess(_ context.Context, now time.Time) ([]ActiveSession, error) {
	// Run ps to find known process names
	out, err := exec.Command("ps", "-eo", "pid,comm").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}

	type procInfo struct {
		pid    int
		source Source
	}

	var procs []procInfo
	for _, line := range bytes.Split(out, []byte("\n")) {
		fields := bytes.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(string(fields[0]))
		if err != nil {
			continue
		}
		// comm may be a full path; take the basename
		comm := string(fields[len(fields)-1])
		base := filepath.Base(comm)
		if src, ok := knownProcesses[base]; ok {
			procs = append(procs, procInfo{pid: pid, source: src})
		}
	}

	if len(procs) == 0 {
		return nil, nil
	}

	claudeDir := d.claudeDir
	if claudeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		claudeDir = filepath.Join(home, ".claude")
	}

	var result []ActiveSession
	for _, p := range procs {
		cwd := getProcessCwd(p.pid)
		if cwd == "" {
			continue
		}

		sessionPath, sessionID := d.findMostRecentSession(claudeDir, cwd)
		if sessionPath == "" {
			continue
		}

		result = append(result, ActiveSession{
			Source:      p.source,
			ProjectPath: cwd,
			SessionPath: sessionPath,
			SessionID:   sessionID,
			DetectedAt:  now,
			Method:      "process",
			PID:         p.pid,
		})
	}

	return result, nil
}

// getProcessCwd returns the current working directory of a process.
// On macOS, uses lsof. On Linux, reads /proc/PID/cwd.
func getProcessCwd(pid int) string {
	pidStr := strconv.Itoa(pid)

	// Try /proc/PID/cwd first (Linux)
	if target, err := os.Readlink("/proc/" + pidStr + "/cwd"); err == nil {
		return target
	}

	// Fall back to lsof (macOS and other Unix)
	out, err := exec.Command("lsof", "-a", "-p", pidStr, "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	// lsof -Fn output: lines starting with 'p' are PIDs, 'f' are FDs, 'n' are names.
	// We want the 'n' line after 'fcwd'.
	lines := bytes.Split(out, []byte("\n"))
	foundCwd := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if line[0] == 'f' && string(line[1:]) == "cwd" {
			foundCwd = true
			continue
		}
		if foundCwd && line[0] == 'n' {
			return string(line[1:])
		}
	}
	return ""
}

// isProcessAlive checks whether a process with the given PID exists.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
