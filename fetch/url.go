package fetch

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// NormalizeTargetURL validates and canonicalizes a URL without resolving its
// hostname. DNS is intentionally left to Zyte.
func NormalizeTargetURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("embedded credentials are not allowed")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("url host is required")
	}
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		return "", fmt.Errorf("non-standard port %q is not allowed", port)
	}
	if host == "localhost" ||
		strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") {
		return "", fmt.Errorf("host %q is not allowed", host)
	}

	if _, err := netip.ParseAddr(host); err == nil {
		return "", fmt.Errorf("ip addresses are not allowed")
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	return parsed.String(), nil
}
