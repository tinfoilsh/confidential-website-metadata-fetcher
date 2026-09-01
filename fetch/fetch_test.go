package fetch

import (
	"context"
	"testing"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/contextdev"
)

type upstreamFetchFunc func(context.Context, string, int64) (*contextdev.Response, error)

func (fn upstreamFetchFunc) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*contextdev.Response, error) {
	return fn(ctx, targetURL, maxBodyBytes)
}

func TestFetchRetrievesPageAndReturnsImageURL(t *testing.T) {
	const (
		pageURL  = "https://example.com/article"
		finalURL = "https://example.com/posts/article"
		imageURL = "https://example.com/images/cover.jpg"
	)
	requestedURLs := []string{}
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, _ int64) (*contextdev.Response, error) {
		requestedURLs = append(requestedURLs, targetURL)
		if targetURL != pageURL {
			t.Fatalf("unexpected upstream URL %q", targetURL)
		}
		return &contextdev.Response{
			URL:         finalURL,
			ContentType: "text/html; charset=utf-8",
			Body: []byte(`<html><head>
					<meta property="og:title" content="Example Article">
					<meta property="og:url" content="https://wrong.example/page">
					<meta property="og:image" content="/images/cover.jpg">
				</head></html>`),
		}, nil
	})
	fetcher := NewFetcher(upstream, 1024)

	result, err := fetcher.Fetch(context.Background(), pageURL)
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	if result.URL != finalURL {
		t.Fatalf("result URL = %q, want %q", result.URL, finalURL)
	}
	if result.Title == nil || *result.Title != "Example Article" {
		t.Fatalf("result title = %v", result.Title)
	}
	if result.Image == nil || *result.Image != imageURL {
		t.Fatalf("image URL = %v, want %q", result.Image, imageURL)
	}
	if len(requestedURLs) != 1 || requestedURLs[0] != pageURL {
		t.Fatalf("upstream requests = %v", requestedURLs)
	}
}
