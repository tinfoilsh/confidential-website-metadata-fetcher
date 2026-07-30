// Package favicon proxies favicon lookups through Zyte so neither the user's
// browser nor the enclave host talks to the upstream icon provider directly.
//
// The upstream is DuckDuckGo's free icon service
// (https://icons.duckduckgo.com/ip3/<host>.ico). Zyte makes the request and the
// response body is returned to the caller with a short-lived in-memory cache
// keyed by hostname.
package favicon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/zyte"
)

const (
	// Upstream template. `%s` is the validated hostname.
	upstreamTemplate = "https://icons.duckduckgo.com/ip3/%s.ico"

	// Cap on the response body we're willing to stream back. Favicons are
	// typically <10KB; this stops a hostile upstream from exhausting
	// memory.
	maxBodyBytes int64 = 256 * 1024
)

// Hostnames follow the usual DNS grammar: letters, digits, hyphens, dots.
// We intentionally forbid `@`, `:`, `/`, `?`, and other URL metacharacters
// so the caller cannot reach arbitrary endpoints via the `host` query
// parameter.
var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

// Entry is what the fetcher caches and what handlers stream back to the
// client. Body is the raw image bytes; ContentType is whatever the
// upstream declared.
type Entry struct {
	Body        []byte
	ContentType string
}

// Fetcher wraps an upstream fetcher and an LRU cache so the public handler can
// be small and testable.
type Fetcher struct {
	upstream upstreamFetcher
	cache    *cache.LRU[Entry]
}

type upstreamFetcher interface {
	Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*zyte.Response, error)
}

// NewFetcher builds a fetcher with sensible defaults for the favicon use
// case.
func NewFetcher(upstream upstreamFetcher, cacheMaxEntries int, cacheTTL time.Duration) *Fetcher {
	return &Fetcher{
		upstream: upstream,
		cache:    cache.New[Entry](cacheMaxEntries, cacheTTL),
	}
}

// ErrInvalidHost signals the caller-supplied hostname failed validation.
// Surface as 400.
var ErrInvalidHost = errors.New("invalid host")

// ErrUpstreamFailed signals the upstream icon service did not return a
// usable image. Surface as 502.
var ErrUpstreamFailed = errors.New("upstream fetch failed")

// Fetch returns the favicon for the given hostname, consulting the cache
// first and falling back to a fresh upstream request.
func (f *Fetcher) Fetch(ctx context.Context, host string) (*Entry, bool, error) {
	host = strings.TrimSpace(strings.ToLower(host))
	if !hostnamePattern.MatchString(host) || len(host) > 253 {
		return nil, false, ErrInvalidHost
	}

	if entry, ok := f.cache.Get(host); ok {
		return &entry, true, nil
	}

	resp, err := f.upstream.Fetch(ctx, fmt.Sprintf(upstreamTemplate, host), maxBodyBytes)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%w: status %d", ErrUpstreamFailed, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/x-icon"
	}

	entry := Entry{Body: resp.Body, ContentType: contentType}
	f.cache.Set(host, entry)
	return &entry, false, nil
}
