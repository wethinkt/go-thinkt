//go:build !darwin && !linux && !windows

package fingerprint

// getSystemFingerprint returns empty for unsupported platforms.
// The generic implementation will fall back to generated fingerprint.
func getSystemFingerprint() Info {
	return Info{}
}
