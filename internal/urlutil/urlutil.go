package urlutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateEndpointURL parses and validates an absolute endpoint URL.
// Only https is allowed for non-local hosts. http is allowed for localhost and loopback IPs.
func ValidateEndpointURL(raw string) (string, error) {
	return validateEndpointURL(raw, false)
}

// ValidateEndpointURLAllowHTTP parses and validates an absolute endpoint URL.
// Both http and https are allowed as long as the URL is otherwise well-formed.
func ValidateEndpointURLAllowHTTP(raw string) (string, error) {
	return validateEndpointURL(raw, true)
}

func validateEndpointURL(raw string, allowRemoteHTTP bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %w", raw, err)
	}
	if !u.IsAbs() {
		return "", fmt.Errorf("url %q must be absolute", raw)
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("url %q must include a host", raw)
	}
	if u.User != nil {
		return "", fmt.Errorf("url %q must not include userinfo", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("url %q must not include query or fragment", raw)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		return u.String(), nil
	case "http":
		if !allowRemoteHTTP && !isLocalHost(u.Hostname()) {
			return "", fmt.Errorf("url %q must use https unless host is localhost or loopback", raw)
		}
		return u.String(), nil
	default:
		return "", fmt.Errorf("url %q must use http or https", raw)
	}
}

func isLocalHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
