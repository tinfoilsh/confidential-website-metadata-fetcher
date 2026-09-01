package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/contextdev"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
)

type httpDoFunc func(*http.Request) (*http.Response, error)

func (fn httpDoFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type upstreamFetchFunc func(context.Context, string, int64) (*contextdev.Response, error)

func (fn upstreamFetchFunc) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*contextdev.Response, error) {
	return fn(ctx, targetURL, maxBodyBytes)
}

func TestFetchFaviconDoesNotCacheResponse(t *testing.T) {
	const (
		faviconURL  = "https://icons.duckduckgo.com/ip3/example.com.ico"
		faviconBody = "\x00\x00\x01\x00icon"
	)
	requestCount := 0
	client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		if req.URL.String() != faviconURL {
			t.Fatalf("request URL = %q, want %q", req.URL.String(), faviconURL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/x-icon"}},
			Body:       io.NopCloser(strings.NewReader(faviconBody)),
			Request:    req,
		}, nil
	})
	server := NewServer(nil, client)

	for range 2 {
		favicon := server.fetchFavicon(context.Background(), "https://example.com/page")
		if favicon.Status != faviconFound || string(favicon.Body) != faviconBody || favicon.ContentType != "image/x-icon" {
			t.Fatalf("unexpected favicon: %+v", favicon)
		}
	}
	if requestCount != 2 {
		t.Fatalf("upstream request count = %d, want 2", requestCount)
	}
}

func TestMetadataEndpointDoesNotCacheResponse(t *testing.T) {
	requestCount := 0
	upstream := upstreamFetchFunc(func(_ context.Context, targetURL string, _ int64) (*contextdev.Response, error) {
		requestCount++
		return &contextdev.Response{
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
		if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
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
	const faviconBody = "\x00\x00\x01\x00icon"
	client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/x-icon"}},
			Body:       io.NopCloser(strings.NewReader(faviconBody)),
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
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
	var response faviconResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != string(faviconFound) || string(response.FaviconBytes) != faviconBody || response.FaviconContentType != "image/x-icon" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestFetchFaviconAcceptsDeclaredSVGImage(t *testing.T) {
	const faviconBody = `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`
	client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/svg+xml"}},
			Body:       io.NopCloser(strings.NewReader(faviconBody)),
			Request:    req,
		}, nil
	})

	result := NewServer(nil, client).fetchFavicon(context.Background(), "https://example.com/page")
	if result.Status != faviconFound || result.ContentType != "image/svg+xml" {
		t.Fatalf("unexpected favicon: %+v", result)
	}
}

func TestFaviconEndpointReturnsLegacyDecodableMissingResponse(t *testing.T) {
	for _, upstreamStatus := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(upstreamStatus), func(t *testing.T) {
			client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: upstreamStatus,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
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
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response["status"] != string(faviconMissing) || response["favicon_bytes"] != "" || response["favicon_content_type"] != "" {
				t.Fatalf("unexpected response: %v", response)
			}
		})
	}
}

func TestFaviconEndpointReturnsRetryableUnavailableResponse(t *testing.T) {
	requestCount := 0
	client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Retry-After": {"120"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})
	server := NewServer(nil, client)
	req := httptest.NewRequest(http.MethodPost, "/favicon", strings.NewReader(`{"url":"https://example.com/page"}`))
	recorder := httptest.NewRecorder()

	server.handleFavicon(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if retryAfter := recorder.Header().Get("Retry-After"); retryAfter != "120" {
		t.Fatalf("Retry-After = %q, want 120", retryAfter)
	}
	var response faviconErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "favicon_unavailable" || !response.Retryable {
		t.Fatalf("unexpected response: %+v", response)
	}
	if requestCount != 1 {
		t.Fatalf("upstream request count = %d, want 1", requestCount)
	}
}

func TestFaviconEndpointReturnsNonRetryableBadGatewayResponse(t *testing.T) {
	tests := map[string]struct {
		status      int
		contentType string
		body        string
	}{
		"malformed success": {status: http.StatusOK, contentType: "text/plain", body: "not an image"},
		"other upstream":    {status: http.StatusBadRequest},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status,
					Header:     http.Header{"Content-Type": {test.contentType}},
					Body:       io.NopCloser(strings.NewReader(test.body)),
					Request:    req,
				}, nil
			})
			server := NewServer(nil, client)
			req := httptest.NewRequest(http.MethodPost, "/favicon", strings.NewReader(`{"url":"https://example.com/page"}`))
			recorder := httptest.NewRecorder()

			server.handleFavicon(recorder, req)
			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
			}
			var response faviconErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Code != "malformed_upstream_response" || response.Retryable {
				t.Fatalf("unexpected response: %+v", response)
			}
		})
	}
}

func TestFetchFaviconClassifiesMalformedSuccessfulResponses(t *testing.T) {
	tests := map[string]struct {
		contentType string
		body        string
	}{
		"empty":     {contentType: "image/png"},
		"oversized": {contentType: "image/png", body: strings.Repeat("x", int(maxFaviconBodyBytes)+1)},
		"non-image": {contentType: "text/plain", body: "not an image"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client := httpDoFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {test.contentType}},
					Body:       io.NopCloser(strings.NewReader(test.body)),
					Request:    req,
				}, nil
			})

			result := NewServer(nil, client).fetchFavicon(context.Background(), "https://example.com/page")
			if result.Status != faviconMalformed {
				t.Fatalf("status = %q, want %q", result.Status, faviconMalformed)
			}
		})
	}
}

func TestFetchFaviconClassifiesTransientFailuresAsUnavailable(t *testing.T) {
	tests := map[string]httpDoFunc{
		"transport error": func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		},
		"timeout": func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		},
		"server error": func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		},
		"rate limit": func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		},
		"request timeout": func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusRequestTimeout,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		},
	}
	for name, client := range tests {
		t.Run(name, func(t *testing.T) {
			result := NewServer(nil, client).fetchFavicon(context.Background(), "https://example.com/page")
			if result.Status != faviconUnavailable {
				t.Fatalf("status = %q, want %q", result.Status, faviconUnavailable)
			}
		})
	}
}

func TestSanitizeRetryAfter(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	validDate := now.Add(time.Hour).Format(http.TimeFormat)
	tests := map[string]struct {
		value string
		want  string
	}{
		"delta seconds": {value: " 120 ", want: "120"},
		"HTTP date":     {value: validDate, want: validDate},
		"invalid":       {value: "later", want: ""},
		"negative":      {value: "-1", want: ""},
		"past date":     {value: now.Add(-time.Hour).Format(http.TimeFormat), want: ""},
		"excessive":     {value: "86401", want: ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := sanitizeRetryAfter(test.value, now); got != test.want {
				t.Fatalf("sanitizeRetryAfter(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestFaviconEndpointRejectsInvalidInput(t *testing.T) {
	server := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/favicon", strings.NewReader(`{"url":"http://localhost/icon"}`))
	recorder := httptest.NewRecorder()

	server.handleFavicon(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
