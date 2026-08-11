package fetch

import (
	"context"
	"testing"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/zyte"
)

type upstreamFetchFunc func(context.Context, string, int64) (*zyte.Response, error)

func (fn upstreamFetchFunc) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*zyte.Response, error) {
	return fn(ctx, targetURL, maxBodyBytes)
}

func TestFetchRetrievesPageAndReturnsImageURL(t *testing.T) {
	const (
		pageURL  = "https://example.com/article"
		finalURL = "https://example.com/posts/article"
		imageURL = "https://example.com/images/cover.jpg"
	)
	requestedURLs := []string{}
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, _ int64) (*zyte.Response, error) {
		requestedURLs = append(requestedURLs, targetURL)
		if targetURL != pageURL {
			t.Fatalf("unexpected upstream URL %q", targetURL)
		}
		return &zyte.Response{
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

func TestFetchSkipsInvalidImageCandidates(t *testing.T) {
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, _ int64) (*zyte.Response, error) {
		return &zyte.Response{
			URL:         targetURL,
			ContentType: "text/html",
			Body: []byte(`<html><head>
				<meta property="og:image" content="%">
				<meta property="og:image" content="javascript:alert(1)">
				<meta property="og:image" content="http://localhost/private.png">
				<meta property="og:image" content="../images/cover.jpg#preview">
			</head></html>`),
		}, nil
	})

	result, err := NewFetcher(upstream, 1024).Fetch(context.Background(), "https://example.com/posts/article")
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	if result.Image == nil || *result.Image != "https://example.com/images/cover.jpg" {
		t.Fatalf("image URL = %v", result.Image)
	}
}

func TestFetchReturnsNilWhenNoImageCandidateIsSafe(t *testing.T) {
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, _ int64) (*zyte.Response, error) {
		return &zyte.Response{
			URL:         targetURL,
			ContentType: "text/html",
			Body: []byte(`<html><head>
				<meta property="og:image" content="ftp://example.com/cover.jpg">
				<meta property="og:image" content="https://127.0.0.1/cover.jpg">
			</head></html>`),
		}, nil
	})

	result, err := NewFetcher(upstream, 1024).Fetch(context.Background(), "https://example.com/article")
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	if result.Image != nil {
		t.Fatalf("image URL = %q, want nil", *result.Image)
	}
}
