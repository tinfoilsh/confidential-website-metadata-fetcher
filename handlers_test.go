package main

import (
	"context"
	"testing"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/zyte"
)

type upstreamFetchFunc func(context.Context, string, int64) (*zyte.Response, error)

func (fn upstreamFetchFunc) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*zyte.Response, error) {
	return fn(ctx, targetURL, maxBodyBytes)
}

func TestFetchFaviconUsesZyteAndCachesResponse(t *testing.T) {
	const faviconURL = "https://icons.duckduckgo.com/ip3/example.com.ico"
	requestCount := 0
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, maxBodyBytes int64) (*zyte.Response, error) {
		requestCount++
		if targetURL != faviconURL || maxBodyBytes != maxFaviconBodyBytes {
			t.Fatalf("unexpected request: url=%q max=%d", targetURL, maxBodyBytes)
		}
		return &zyte.Response{
			ContentType: "image/x-icon",
			Body:        []byte("icon"),
		}, nil
	})
	server := NewServer(nil, upstream, 10, time.Minute)

	for range 2 {
		favicon := server.fetchFavicon(context.Background(), "https://example.com/page")
		if string(favicon.Body) != "icon" || favicon.ContentType != "image/x-icon" {
			t.Fatalf("unexpected favicon: %+v", favicon)
		}
	}
	if requestCount != 1 {
		t.Fatalf("upstream request count = %d, want 1", requestCount)
	}
}
