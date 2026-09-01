package fetch

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"strings"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/contextdev"
)

// Result is the extracted metadata returned to callers. Text fields are
// pointers so the JSON response can distinguish "missing" (null) from "empty
// string".
type Result struct {
	URL         string  `json:"url"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	SiteName    *string `json:"site_name"`
	Image       *string `json:"image"`
}

// Fetcher returns page metadata extracted server-side by context.dev.
type Fetcher struct {
	maxBodyBytes int64
	upstream     contextdev.Fetcher
}

// NewFetcher returns a Fetcher that retrieves upstream resources through
// context.dev.
func NewFetcher(upstream contextdev.Fetcher, maxBodyBytes int64) *Fetcher {
	return &Fetcher{maxBodyBytes: maxBodyBytes, upstream: upstream}
}

// Fetch resolves the page and returns its metadata. Any error is suitable to
// report to callers; detailed error information is left to the caller's log.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Result, error) {
	page, err := f.upstream.Fetch(ctx, rawURL, f.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	contentType, _, err := mime.ParseMediaType(page.ContentType)
	if err != nil || !strings.EqualFold(contentType, "text/html") {
		return nil, fmt.Errorf("fetch metadata: content type must be text/html")
	}

	pageURL := page.URL
	if pageURL == "" {
		pageURL = rawURL
	}
	pageURL, err = NormalizeTargetURL(pageURL)
	if err != nil {
		return nil, fmt.Errorf("normalize final URL: %w", err)
	}
	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("parse final URL: %w", err)
	}

	result := &Result{URL: pageURL}
	if title := strings.TrimSpace(page.Title); title != "" {
		result.Title = &title
	}
	if desc := strings.TrimSpace(page.Description); desc != "" {
		result.Description = &desc
	}
	if site := strings.TrimSpace(page.SiteName); site != "" {
		result.SiteName = &site
	}
	if imageValue := strings.TrimSpace(page.Image); imageValue != "" {
		imageRef, err := url.Parse(imageValue)
		if err == nil {
			image, err := NormalizeTargetURL(baseURL.ResolveReference(imageRef).String())
			if err == nil {
				result.Image = &image
			}
		}
	}
	return result, nil
}
