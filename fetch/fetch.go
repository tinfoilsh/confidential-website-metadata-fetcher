package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/otiai10/opengraph/v2"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/config"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/zyte"
)

// Result is the extracted metadata returned to callers. Text fields are
// pointers so the JSON response can distinguish "missing" (null) from "empty
// string". The favicon is intentionally absent here — callers always receive
// favicon bytes inlined in the HTTP response so no client ever has to GET a
// third-party favicon URL.
type Result struct {
	URL         string  `json:"url"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	SiteName    *string `json:"site_name"`
	Image       *string `json:"image"`
}

// Fetcher extracts Open Graph metadata from resources retrieved through Zyte.
type Fetcher struct {
	cfg      *config.Config
	upstream upstreamFetcher
}

type upstreamFetcher interface {
	Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*zyte.Response, error)
}

// NewFetcher returns a Fetcher that retrieves upstream resources through Zyte.
func NewFetcher(cfg *config.Config, upstream upstreamFetcher) *Fetcher {
	return &Fetcher{cfg: cfg, upstream: upstream}
}

// Fetch resolves the page and returns its metadata. Any error is suitable to
// report to callers; detailed error information is left to the caller's log.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Result, error) {
	if err := ValidateTargetURL(rawURL); err != nil {
		return nil, &ClientError{msg: err.Error()}
	}

	page, err := f.upstream.Fetch(ctx, rawURL, f.cfg.MaxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	if page.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch metadata: status %d", page.StatusCode)
	}
	if !strings.HasPrefix(page.Header.Get("Content-Type"), "text/html") {
		return nil, fmt.Errorf("fetch metadata: content type must be text/html")
	}

	ogp := opengraph.New(page.URL)
	if err := ogp.Parse(bytes.NewReader(page.Body)); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	// Resolve relative og:image URLs against the final page URL before fetching.
	if err := ogp.ToAbs(); err != nil {
		return nil, fmt.Errorf("resolve metadata URLs: %w", err)
	}

	result := &Result{URL: ogp.URL}
	if result.URL == "" {
		result.URL = page.URL
	}
	if result.URL == "" {
		result.URL = rawURL
	}
	if title := strings.TrimSpace(ogp.Title); title != "" {
		result.Title = &title
	}
	if desc := strings.TrimSpace(ogp.Description); desc != "" {
		result.Description = &desc
	}
	if site := strings.TrimSpace(ogp.SiteName); site != "" {
		result.SiteName = &site
	}
	if len(ogp.Image) > 0 && strings.TrimSpace(ogp.Image[0].URL) != "" {
		image := strings.TrimSpace(ogp.Image[0].URL)
		result.Image = &image
	}
	return result, nil
}

// ClientError signals that the request was rejected because of caller input
// (invalid URL, blocked host, etc.), not an upstream failure.
type ClientError struct{ msg string }

func (e *ClientError) Error() string { return e.msg }

// IsClientError reports whether err originated from caller input.
func IsClientError(err error) bool {
	var ce *ClientError
	return errors.As(err, &ce)
}
