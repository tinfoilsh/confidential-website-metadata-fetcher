package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/zyte"
)

type httpDoFunc func(*http.Request) (*http.Response, error)

func (fn httpDoFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type upstreamFetchFunc func(context.Context, string, int64) (*zyte.Response, error)

func (fn upstreamFetchFunc) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*zyte.Response, error) {
	return fn(ctx, targetURL, maxBodyBytes)
}

func TestFetchFaviconDoesNotCacheResponse(t *testing.T) {
	const faviconURL = "https://icons.duckduckgo.com/ip3/example.com.ico"
	requestCount := 0
	client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		if req.URL.String() != faviconURL {
			t.Fatalf("request URL = %q, want %q", req.URL.String(), faviconURL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/x-icon"}},
			Body:       io.NopCloser(strings.NewReader("icon")),
			Request:    req,
		}, nil
	})
	server := NewServer(nil, client)

	for range 2 {
		favicon := server.fetchFavicon(context.Background(), "https://example.com/page")
		if string(favicon.Body) != "icon" || favicon.ContentType != "image/x-icon" {
			t.Fatalf("unexpected favicon: %+v", favicon)
		}
	}
	if requestCount != 2 {
		t.Fatalf("upstream request count = %d, want 2", requestCount)
	}
}

func TestMetadataEndpointDoesNotCacheResponse(t *testing.T) {
	requestCount := 0
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, _ int64) (*zyte.Response, error) {
		requestCount++
		return &zyte.Response{
			URL:         targetURL,
			ContentType: "text/html",
			Body:        []byte(`<meta property="og:title" content="Example">`),
		}, nil
	})
	server := NewServer(fetch.NewFetcher(upstream, 1024), nil)

	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/metadata", strings.NewReader(`{"url":"https://example.com/page"}`))
		recorder := httptest.NewRecorder()
		server.handleMetadata(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if body := recorder.Body.String(); !strings.Contains(body, `"cached":false`) {
			t.Fatalf("response body = %s", body)
		}
	}
	if requestCount != 2 {
		t.Fatalf("upstream request count = %d, want 2", requestCount)
	}
}

func TestFaviconEndpointReturnsInlineIconWithoutMetadataFetcher(t *testing.T) {
	client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/x-icon"}},
			Body:       io.NopCloser(strings.NewReader("icon")),
			Request:    req,
		}, nil
	})
	server := NewServer(nil, client)
	req := httptest.NewRequest(http.MethodPost, "/favicon", strings.NewReader(`{"url":"https://example.com/page"}`))
	recorder := httptest.NewRecorder()

	server.handleFavicon(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"favicon_bytes":"aWNvbg=="`) {
		t.Fatalf("response body = %s", body)
	}
}
