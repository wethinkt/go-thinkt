//go:build windows

package thinkt

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// detectProcess on Windows is not yet implemented.
// TODO: Implement using tasklist/wmic to find AI tool processes and their cwds.
func (d *ActiveSessionDetector) detectProcess(_ context.Context, _ time.Time) ([]ActiveSession, error) {
	return nil, nil
}

// isProcessAlive checks whether a process with the given PID exists on Windows
// by running tasklist and checking if the PID appears.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH").Output()
	if err != nil {
		return false
	}
	// tasklist returns "INFO: No tasks are running..." if PID not found
	return !strings.Contains(string(out), "No tasks")
}
