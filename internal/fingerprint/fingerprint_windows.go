//go:build windows

package fingerprint

import (
	"golang.org/x/sys/windows/registry"
	"strings"
)

// getSystemFingerprint tries to get the machine fingerprint on Windows.
// It uses the MachineGuid from the registry.
func getSystemFingerprint() Info {
	// Try MachineGuid from registry
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`,
		registry.QUERY_VALUE)
	if err == nil {
		defer key.Close()

		guid, _, err := key.GetStringValue("MachineGuid")
		if err == nil && guid != "" {
			return Info{
				Fingerprint: normalizeFingerprint(guid),
				Source:      "MachineGuid",
				Path:        `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`,
				Components:  []string{guid},
			}
		}
	}

	// Try ProductId as fallback
	key, err = registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE)
	if err == nil {
		defer key.Close()

		productID, _, err := key.GetStringValue("ProductId")
		if err == nil && productID != "" {
			return Info{
				Fingerprint: normalizeFingerprint(productID),
				Source:      "Windows ProductId",
				Path:        `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProductId`,
				Components:  []string{productID},
			}
		}
	}

	return Info{}
}
