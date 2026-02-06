//go:build linux

package fingerprint

import (
	"os"
	"strings"
)

// getSystemFingerprint tries to get the machine fingerprint on Linux.
// It tries multiple sources in order of preference.
func getSystemFingerprint() Info {
	// Try systemd machine-id first (most reliable)
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" && id != "00000000000000000000000000000000" {
			return Info{
				Fingerprint: normalizeFingerprint(id),
				Source:      "machine-id",
				Path:        "/etc/machine-id",
				Components:  []string{id},
			}
		}
	}

	// Try D-Bus machine-id (older systems)
	if data, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" && id != "00000000000000000000000000000000" {
			return Info{
				Fingerprint: normalizeFingerprint(id),
				Source:      "dbus-machine-id",
				Path:        "/var/lib/dbus/machine-id",
				Components:  []string{id},
			}
		}
	}

	// Try product_uuid (requires root usually, but worth trying)
	if data, err := os.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return Info{
				Fingerprint: normalizeFingerprint(id),
				Source:      "product-uuid",
				Path:        "/sys/class/dmi/id/product_uuid",
				Components:  []string{id},
			}
		}
	}

	return Info{}
}
