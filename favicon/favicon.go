// Package favicon proxies favicon lookups through the enclave so the
// user's browser never talks to the upstream icon provider directly.
//
// The upstream is DuckDuckGo's free icon service
// (https://icons.duckduckgo.com/ip3/<host>.ico). Requests are made
// server-side from inside the CVM; the response body is streamed back to
// the caller with a short-lived in-memory cache keyed by hostname.
package favicon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
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

// Fetcher wraps an HTTP client and an LRU cache so the public handler can
// be small and testable.
type Fetcher struct {
	client *http.Client
	cache  *cache.LRU[Entry]
}

// NewFetcher builds a fetcher with sensible defaults for the favicon use
// case.
func NewFetcher(timeout time.Duration, cacheMaxEntries int, cacheTTL time.Duration) *Fetcher {
	return &Fetcher{
		client: &http.Client{Timeout: timeout},
		cache:  cache.New[Entry](cacheMaxEntries, cacheTTL),
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

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf(upstreamTemplate, host),
		nil,
	)
	if err != nil {
		return nil, false, fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MetadataFetchBot/1.0)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%w: status %d", ErrUpstreamFailed, resp.StatusCode)
	}

	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, io.LimitReader(resp.Body, maxBodyBytes+1)); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
	}
	if int64(buf.Len()) > maxBodyBytes {
		return nil, false, fmt.Errorf("%w: body too large", ErrUpstreamFailed)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/x-icon"
	}

	entry := Entry{Body: buf.Bytes(), ContentType: contentType}
	f.cache.Set(host, entry)
	return &entry, false, nil
}
