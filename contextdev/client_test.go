package contextdev

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/context-dot-dev/context-go-sdk/option"
)

func TestFetchReturnsDecodedUpstreamResponse(t *testing.T) {
	const (
		apiKey    = "test-api-key"
		targetURL = "https://example.com/article"
		pageHTML  = `<html><head><title>Example</title></head></html>`
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+apiKey {
			t.Fatalf("Authorization = %q, want bearer token", auth)
		}
		if !strings.HasSuffix(r.URL.Path, "/web/scrape/html") {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("url") != targetURL {
			t.Fatalf("url param = %q, want %q", query.Get("url"), targetURL)
		}
		if query.Get("maxAgeMs") != "0" {
			t.Fatalf("maxAgeMs param = %q, want 0", query.Get("maxAgeMs"))
		}
		if query.Get("zdr") != "enabled" {
			t.Fatalf("zdr param = %q, want enabled", query.Get("zdr"))
		}
		writeJSON(t, w, map[string]any{
			"success": true,
			"html":    pageHTML,
			"url":     targetURL,
			"type":    "html",
			"metadata": map[string]any{
				"sourceUrl":   targetURL,
				"finalUrl":    "https://example.com/final",
				"title":       "Example Article",
				"description": "A short summary.",
				"siteName":    "Example",
				"image":       "https://example.com/cover.jpg",
			},
			"cache_metadata": map[string]any{"status": "zdr", "age_ms": 0},
		})
	}))
	defer server.Close()

	client := NewClient(apiKey, time.Second, option.WithBaseURL(server.URL))
	response, err := client.Fetch(context.Background(), targetURL, 1024)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if response.URL != "https://example.com/final" {
		t.Fatalf("response URL = %q", response.URL)
	}
	if response.ContentType != "text/html" {
		t.Fatalf("content type = %q", response.ContentType)
	}
	if response.Title != "Example Article" {
		t.Fatalf("title = %q", response.Title)
	}
	if response.Description != "A short summary." {
		t.Fatalf("description = %q", response.Description)
	}
	if response.SiteName != "Example" {
		t.Fatalf("site name = %q", response.SiteName)
	}
	if response.Image != "https://example.com/cover.jpg" {
		t.Fatalf("image = %q", response.Image)
	}
}

func TestFetchRejectsAPIResponseExceedingReadLimit(t *testing.T) {
	const maxBodyBytes = 4
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"success": true,
			"html":    strings.Repeat("x", int(maxBodyBytes*apiResponseSizeMultiplier)+maxAPIResponseOverhead+1024),
			"url":     "https://example.com",
			"type":    "html",
			"metadata": map[string]any{
				"sourceUrl": "https://example.com",
				"finalUrl":  "https://example.com",
			},
			"cache_metadata": map[string]any{"status": "zdr", "age_ms": 0},
		})
	}))
	defer server.Close()

	client := NewClient("test-api-key", time.Second, option.WithBaseURL(server.URL), option.WithMaxRetries(0))
	if _, err := client.Fetch(context.Background(), "https://example.com", maxBodyBytes); err == nil {
		t.Fatal("expected read limit error for oversized API response")
	}
}

func TestLimitedReadCloserRejectsBodyExceedingLimitAtEOF(t *testing.T) {
	const maxBytes = 4
	body := &limitedReadCloser{
		inner:     io.NopCloser(strings.NewReader(strings.Repeat("x", maxBytes+1))),
		remaining: maxBytes + 1,
	}

	if _, err := io.ReadAll(body); err == nil {
		t.Fatal("expected size error for body exceeding the limit at EOF")
	}
}

func TestLimitedReadCloserAcceptsBodyAtLimit(t *testing.T) {
	const maxBytes = 4
	body := &limitedReadCloser{
		inner:     io.NopCloser(strings.NewReader(strings.Repeat("x", maxBytes))),
		remaining: maxBytes + 1,
	}

	read, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body at limit: %v", err)
	}
	if len(read) != maxBytes {
		t.Fatalf("read %d bytes, want %d", len(read), maxBytes)
	}
}

func TestFetchRejectsBodyLimitThatWouldOverflow(t *testing.T) {
	client := NewClient("test-api-key", time.Second)
	if _, err := client.Fetch(context.Background(), "https://example.com", maxSupportedBodyBytes+1); err == nil {
		t.Fatal("expected invalid body limit error")
	}
}

func TestFetchReportsAPIErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		if _, err := w.Write([]byte(`{"message":"ZDR not enabled","error_code":"FORBIDDEN"}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient("test-api-key", time.Second, option.WithBaseURL(server.URL), option.WithMaxRetries(0))
	_, err := client.Fetch(context.Background(), "https://example.com", 1024)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v, want status 403", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
