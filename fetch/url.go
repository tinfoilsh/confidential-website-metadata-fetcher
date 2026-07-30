package fetch

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

const (
	allowedSchemeHTTP  = "http"
	allowedSchemeHTTPS = "https"
)

var blockedHostSuffixes = []string{
	".internal",
	".local",
	".localhost",
}

// ValidateTargetURL rejects URLs that cannot be safely passed to Zyte. DNS is
// intentionally left to Zyte so the enclave never resolves the target host.
func ValidateTargetURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if parsed.User != nil {
		return fmt.Errorf("embedded credentials are not allowed")
	}
	if parsed.Scheme != allowedSchemeHTTP && parsed.Scheme != allowedSchemeHTTPS {
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		return fmt.Errorf("non-standard port %q is not allowed", port)
	}
	if isBlockedHostname(host) {
		return fmt.Errorf("host %q is not allowed", host)
	}

	if _, err := netip.ParseAddr(host); err == nil {
		return fmt.Errorf("ip addresses are not allowed")
	}
	return nil
}

func isBlockedHostname(host string) bool {
	if host == "localhost" {
		return true
	}
	for _, suffix := range blockedHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}
