package server

import "os"

// DefaultAPIAuthConfig returns a default API auth configuration.
// Uses THINKT_API_TOKEN env var if set, otherwise no auth.
func DefaultAPIAuthConfig() AuthConfig {
	if os.Getenv("THINKT_API_TOKEN") != "" {
		return AuthConfig{
			Mode:   AuthModeEnvToken,
			EnvVar: "THINKT_API_TOKEN",
			Realm:  "thinkt-api",
		}
	}
	return AuthConfig{Mode: AuthModeNone, Realm: "thinkt-api"}
}
