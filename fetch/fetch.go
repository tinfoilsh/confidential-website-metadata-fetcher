package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/otiai10/opengraph/v2"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/config"
)

// Result is the extracted metadata returned to callers. All fields except
// URL are pointers so the JSON response can distinguish "missing" (null)
// from "empty string".
type Result struct {
	URL         string  `json:"url"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	SiteName    *string `json:"site_name"`
	Image       *string `json:"image"`
	Favicon     *string `json:"favicon"`
}

// Fetcher extracts Open Graph metadata from a URL using an SSRF-hardened
// HTTP client.
type Fetcher struct {
	cfg    *config.Config
	client *http.Client
}

// NewFetcher returns a Fetcher whose HTTP client refuses to follow redirects
// into private/loopback/reserved addresses and enforces the configured body
// size and redirect caps.
func NewFetcher(cfg *config.Config) *Fetcher {
	client := &http.Client{
		Timeout: cfg.FetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= cfg.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", cfg.MaxRedirects)
			}
			return ValidateTargetURL(req.Context(), req.URL.String())
		},
	}
	return &Fetcher{cfg: cfg, client: client}
}

// Fetch resolves the page and returns its metadata. Any error is suitable to
// report to callers; detailed error information is left to the caller's log.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Result, error) {
	if err := ValidateTargetURL(ctx, rawURL); err != nil {
		return nil, &ClientError{msg: err.Error()}
	}

	ogp, err := opengraph.Fetch(rawURL, opengraph.Intent{
		Context:    ctx,
		HTTPClient: f.client,
		Headers: map[string]string{
			"User-Agent":      f.cfg.UserAgent,
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "en-US,en;q=0.9",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}

	// Resolve relative og:image URLs against the final page URL so callers
	// always get an absolute link they can render.
	ogp.ToAbs()

	result := &Result{URL: ogp.URL}
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
		img := strings.TrimSpace(ogp.Image[0].URL)
		result.Image = &img
	}
	if favicon := strings.TrimSpace(ogp.Favicon.URL); favicon != "" {
		result.Favicon = &favicon
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
