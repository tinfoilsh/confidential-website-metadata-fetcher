package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type httpDoFunc func(*http.Request) (*http.Response, error)

func (fn httpDoFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestFetchFaviconUsesDuckDuckGoAndCachesResponse(t *testing.T) {
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
	server := NewServer(nil, client, 10, time.Minute)

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

func TestFaviconEndpointReturnsInlineIconWithoutMetadataFetcher(t *testing.T) {
	client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/x-icon"}},
			Body:       io.NopCloser(strings.NewReader("icon")),
			Request:    req,
		}, nil
	})
	server := NewServer(nil, client, 10, time.Minute)
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
