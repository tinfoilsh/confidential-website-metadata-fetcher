package fetch

import (
	"context"
	"fmt"
	"net"
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

// blockedIPPrefixes covers every CIDR we do not want the scraper to reach.
// Mirrors the defensive list used by confidential-websearch so operators get
// the same SSRF guarantees across services.
var blockedIPPrefixes = mustParsePrefixes([]string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"2001:db8::/32",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
})

// ValidateTargetURL rejects URLs that point at private, loopback, link-local,
// multicast, or otherwise reserved addresses so the service cannot be used as
// an SSRF vector. It performs DNS resolution so attacker-controlled public
// hostnames that resolve to internal ranges are also blocked.
func ValidateTargetURL(ctx context.Context, rawURL string) error {
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

	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlockedAddr(addr) {
			return fmt.Errorf("ip address %q is not allowed", host)
		}
		return nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("host %q resolved to no addresses", host)
	}
	for _, addr := range addrs {
		if ip, ok := netip.AddrFromSlice(addr.IP); ok && isBlockedAddr(ip) {
			return fmt.Errorf("resolved address %q is not allowed", addr.IP.String())
		}
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

func isBlockedAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsMulticast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsUnspecified() {
		return true
	}
	for _, prefix := range blockedIPPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func mustParsePrefixes(prefixes []string) []netip.Prefix {
	parsed := make([]netip.Prefix, 0, len(prefixes))
	for _, prefix := range prefixes {
		p, err := netip.ParsePrefix(prefix)
		if err != nil {
			panic(fmt.Sprintf("invalid blocked IP prefix %q: %v", prefix, err))
		}
		parsed = append(parsed, p)
	}
	return parsed
}
