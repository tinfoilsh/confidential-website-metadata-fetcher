// Package contextdev fetches upstream pages through the context.dev web
// scraping API (https://docs.context.dev/api-reference/web-scraping/html) so
// the enclave never contacts target hosts directly.
package contextdev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	sdk "github.com/context-dot-dev/context-go-sdk"
	"github.com/context-dot-dev/context-go-sdk/option"
)

const (
	apiResponseSizeMultiplier = 2
	maxAPIResponseOverhead    = 64 * 1024
	maxSupportedBodyBytes     = (math.MaxInt64 - maxAPIResponseOverhead - 1) / apiResponseSizeMultiplier
)

type Client struct {
	sdk     sdk.Client
	timeout time.Duration
}

// Response carries the final URL and the page metadata that context.dev
// extracts server-side, so callers never parse page HTML themselves.
type Response struct {
	URL         string
	ContentType string
	Title       string
	Description string
	SiteName    string
	Image       string
}

type Fetcher interface {
	Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*Response, error)
}

// NewClient returns a Client backed by the official context.dev Go SDK. Extra
// request options (for example option.WithBaseURL in tests) apply to every
// request the client makes.
func NewClient(apiKey string, timeout time.Duration, opts ...option.RequestOption) *Client {
	return &Client{
		sdk: sdk.NewClient(append([]option.RequestOption{
			option.WithAPIKey(apiKey),
		}, opts...)...),
		timeout: timeout,
	}
}

// Fetch scrapes targetURL through context.dev with zero data retention
// enforced: zdr=enabled keeps the request out of shared caches and retained
// usage logs, and maxAgeMs=0 guarantees a fresh scrape. Requests fail with
// ZDR_NOT_ENABLED until zero data retention is enabled for the organization
// (contact support@context.dev).
func (c *Client) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*Response, error) {
	if maxBodyBytes <= 0 || maxBodyBytes > maxSupportedBodyBytes {
		return nil, fmt.Errorf("invalid maximum body size")
	}
	maxAPIResponseBytes := maxBodyBytes*apiResponseSizeMultiplier + maxAPIResponseOverhead

	scraped, err := c.sdk.Web.WebScrapeHTML(ctx, sdk.WebWebScrapeHTMLParams{
		URL:      targetURL,
		MaxAgeMs: sdk.Int(0),
	},
		option.WithQuery("zdr", "enabled"),
		option.WithHTTPClient(&limitedHTTPClient{
			inner:    &http.Client{Timeout: c.timeout},
			maxBytes: maxAPIResponseBytes,
		}),
	)
	if err != nil {
		var apiErr *sdk.Error
		if errors.As(err, &apiErr) {
			// Deliberately not wrapping apiErr: its Error() string includes
			// the full request URL (carrying the confidential target URL) and
			// raw response JSON, which must not propagate into logs.
			return nil, fmt.Errorf("context.dev request failed with status %d", apiErr.StatusCode)
		}
		return nil, fmt.Errorf("context.dev request failed: %w", err)
	}
	finalURL := scraped.Metadata.FinalURL
	if finalURL == "" {
		finalURL = scraped.URL
	}
	return &Response{
		URL:         finalURL,
		ContentType: contentTypeFor(scraped.Type),
		Title:       scraped.Metadata.Title,
		Description: scraped.Metadata.Description,
		SiteName:    scraped.Metadata.SiteName,
		Image:       scraped.Metadata.Image,
	}, nil
}

// contentTypeFor maps the API's detected content type to a MIME type so
// downstream consumers can keep filtering on standard media types.
func contentTypeFor(detected sdk.WebWebScrapeHTMLResponseType) string {
	switch detected {
	case sdk.WebWebScrapeHTMLResponseTypeHTML:
		return "text/html"
	case sdk.WebWebScrapeHTMLResponseTypeXml:
		return "application/xml"
	case sdk.WebWebScrapeHTMLResponseTypeJson:
		return "application/json"
	case sdk.WebWebScrapeHTMLResponseTypeText:
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// limitedHTTPClient bounds how many response bytes the SDK may read so a
// misbehaving upstream cannot exhaust enclave memory.
type limitedHTTPClient struct {
	inner    *http.Client
	maxBytes int64
}

func (c *limitedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.inner.Do(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	// Allow one extra byte so a response of exactly maxBytes still reaches
	// EOF while anything larger fails on the next read.
	resp.Body = &limitedReadCloser{inner: resp.Body, remaining: c.maxBytes + 1}
	return resp, nil
}

type limitedReadCloser struct {
	inner     io.ReadCloser
	remaining int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		return 0, fmt.Errorf("context.dev response too large")
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.inner.Read(p)
	l.remaining -= int64(n)
	if l.remaining <= 0 {
		// The sentinel byte was consumed, so the response exceeds maxBytes
		// even when this read also reported EOF.
		return n, fmt.Errorf("context.dev response too large")
	}
	return n, err
}

func (l *limitedReadCloser) Close() error { return l.inner.Close() }
