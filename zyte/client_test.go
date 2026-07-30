package zyte

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchReturnsDecodedUpstreamResponse(t *testing.T) {
	const (
		apiKey    = "test-api-key"
		targetURL = "https://example.com/article"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != apiKey || password != "" {
			t.Fatal("request did not contain expected basic authentication")
		}
		var request extractRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.URL != targetURL || !request.HTTPResponseBody || !request.HTTPResponseHeaders {
			t.Fatalf("unexpected request: %+v", request)
		}
		writeJSON(t, w, extractResponse{
			URL:              "https://example.com/final",
			StatusCode:       http.StatusOK,
			HTTPResponseBody: []byte("response-body"),
			HTTPResponseHeaders: []httpHeader{
				{Name: "Content-Type", Value: "text/html"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(apiKey, time.Second)
	client.endpoint = server.URL
	response, err := client.Fetch(context.Background(), targetURL, 1024)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if response.URL != "https://example.com/final" {
		t.Fatalf("response URL = %q", response.URL)
	}
	if string(response.Body) != "response-body" {
		t.Fatalf("response body = %q", response.Body)
	}
	if response.ContentType != "text/html" {
		t.Fatalf("content type = %q", response.ContentType)
	}
}

func TestFetchRejectsOversizedUpstreamBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, extractResponse{
			URL:              "https://example.com",
			StatusCode:       http.StatusOK,
			HTTPResponseBody: []byte("too-large"),
		})
	}))
	defer server.Close()

	client := NewClient("test-api-key", time.Second)
	client.endpoint = server.URL
	if _, err := client.Fetch(context.Background(), "https://example.com", 4); err == nil {
		t.Fatal("expected oversized upstream body error")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
