//go:build darwin

package fingerprint

import (
	"os/exec"
	"strings"
)

// getSystemFingerprint tries to get the machine fingerprint on macOS.
// It uses IOPlatformUUID from ioreg as the primary source.
func getSystemFingerprint() Info {
	// Try ioreg for IOPlatformUUID
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	out, err := cmd.Output()
	if err == nil {
		// Parse output for IOPlatformUUID
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "IOPlatformUUID") {
				// Extract UUID from "IOPlatformUUID" = "XXXX-XXXX-..."
				parts := strings.Split(line, "=\"")
				if len(parts) >= 2 {
					uuid := strings.Trim(parts[1], "\"")
					if uuid != "" {
						return Info{
							Fingerprint: normalizeFingerprint(uuid),
							Source:      "IOPlatformUUID",
							Path:        "ioreg -rd1 -c IOPlatformExpertDevice",
							Components:  []string{uuid},
						}
					}
				}
			}
		}
	}

	// Fallback to system_profiler (slower but more reliable)
	cmd = exec.Command("system_profiler", "SPHardwareDataType")
	out, err = cmd.Output()
	if err == nil {
		// Look for Hardware UUID
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Hardware UUID") {
				parts := strings.Split(line, ": ")
				if len(parts) >= 2 {
					uuid := strings.TrimSpace(parts[1])
					if uuid != "" {
						return Info{
							Fingerprint: normalizeFingerprint(uuid),
							Source:      "Hardware UUID",
							Path:        "system_profiler SPHardwareDataType",
							Components:  []string{uuid},
						}
					}
				}
			}
		}
	}

	return Info{}
}
