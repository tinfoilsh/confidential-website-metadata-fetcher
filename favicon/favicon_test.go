package favicon

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/zyte"
)

type upstreamFetchFunc func(context.Context, string, int64) (*zyte.Response, error)

func (fn upstreamFetchFunc) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*zyte.Response, error) {
	return fn(ctx, targetURL, maxBodyBytes)
}

func TestFetchRetrievesDuckDuckGoIconThroughUpstreamAndCachesIt(t *testing.T) {
	const expectedURL = "https://icons.duckduckgo.com/ip3/example.com.ico"
	requestCount := 0
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, requestedMaxBodyBytes int64) (*zyte.Response, error) {
		requestCount++
		if targetURL != expectedURL {
			t.Fatalf("target URL = %q, want %q", targetURL, expectedURL)
		}
		if requestedMaxBodyBytes != maxBodyBytes {
			t.Fatalf("max body bytes = %d, want %d", requestedMaxBodyBytes, maxBodyBytes)
		}
		return &zyte.Response{
			URL:        expectedURL,
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/x-icon"}},
			Body:       []byte("icon-data"),
		}, nil
	})
	fetcher := NewFetcher(upstream, 10, time.Minute)

	first, cached, err := fetcher.Fetch(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if cached || string(first.Body) != "icon-data" {
		t.Fatalf("unexpected first response: cached=%v body=%q", cached, first.Body)
	}
	second, cached, err := fetcher.Fetch(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if !cached || string(second.Body) != "icon-data" {
		t.Fatalf("unexpected cached response: cached=%v body=%q", cached, second.Body)
	}
	if requestCount != 1 {
		t.Fatalf("upstream request count = %d, want 1", requestCount)
	}
}
