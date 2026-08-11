package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
)

const (
	maxRequestBodyBytes int64 = 64 * 1024
	maxFaviconBodyBytes int64 = 256 * 1024
	maxRetryAfter             = 24 * time.Hour
	faviconURLTemplate        = "https://icons.duckduckgo.com/ip3/%s.ico"
)

type metadataRequest struct {
	URL string `json:"url"`
}

type faviconResponse struct {
	Status             string `json:"status"`
	FaviconBytes       []byte `json:"favicon_bytes"`
	FaviconContentType string `json:"favicon_content_type"`
}

type metadataResponse struct {
	fetch.Result
	Cached bool `json:"cached"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type faviconErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
}

// Server wires the fetchers and HTTP handlers together so main.go can stand the
// service up with one call.
type Server struct {
	fetcher       *fetch.Fetcher
	faviconClient httpDoer
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewServer(
	fetcher *fetch.Fetcher,
	faviconClient httpDoer,
) *Server {
	return &Server{
		fetcher:       fetcher,
		faviconClient: faviconClient,
	}
}

type faviconEntry struct {
	Body        []byte
	ContentType string
	Status      faviconStatus
	RetryAfter  string
}

type faviconStatus string

const (
	faviconFound       faviconStatus = "found"
	faviconMissing     faviconStatus = "missing"
	faviconUnavailable faviconStatus = "unavailable"
	faviconMalformed   faviconStatus = "malformed"
)

// Routes registers the service endpoints on the given mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/metadata", s.handleMetadata)
	mux.HandleFunc("/favicon", s.handleFavicon)
	mux.HandleFunc("/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	targetURL, ok := decodeTargetURL(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	result, err := s.fetcher.Fetch(ctx, targetURL)
	if err != nil {
		log.Print("metadata fetch failed")
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch metadata"})
		return
	}

	writeJSON(w, http.StatusOK, metadataResponse{
		Result: *result,
		Cached: false,
	})
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	targetURL, ok := decodeTargetURL(w, r)
	if !ok {
		return
	}
	favicon := s.fetchFavicon(r.Context(), targetURL)
	switch favicon.Status {
	case faviconFound:
		writeJSON(w, http.StatusOK, faviconResponse{
			Status:             string(favicon.Status),
			FaviconBytes:       favicon.Body,
			FaviconContentType: favicon.ContentType,
		})
	case faviconMissing:
		writeJSON(w, http.StatusOK, faviconResponse{
			Status:             string(favicon.Status),
			FaviconBytes:       []byte{},
			FaviconContentType: "",
		})
	case faviconUnavailable:
		if retryAfter := sanitizeRetryAfter(favicon.RetryAfter, time.Now()); retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		writeJSON(w, http.StatusServiceUnavailable, faviconErrorResponse{
			Error:     "favicon temporarily unavailable",
			Code:      "favicon_unavailable",
			Retryable: true,
		})
	default:
		writeJSON(w, http.StatusBadGateway, faviconErrorResponse{
			Error:     "invalid favicon response from upstream",
			Code:      "malformed_upstream_response",
			Retryable: false,
		})
	}
}

func decodeTargetURL(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req metadataRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return "", false
	}
	targetURL, err := fetch.NormalizeTargetURL(req.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return "", false
	}
	return targetURL, true
}

// fetchFavicon retrieves the favicon from DuckDuckGo so clients never reach an
// external icon host directly.
func (s *Server) fetchFavicon(ctx context.Context, pageURL string) faviconEntry {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return faviconEntry{Status: faviconMalformed}
	}
	host := parsed.Hostname()
	if host == "" {
		return faviconEntry{Status: faviconMalformed}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf(faviconURLTemplate, url.PathEscape(host)),
		nil,
	)
	if err != nil {
		return faviconEntry{Status: faviconMalformed}
	}
	resp, err := s.faviconClient.Do(req)
	if err != nil {
		return faviconEntry{Status: faviconUnavailable}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return faviconEntry{Status: faviconMissing}
	}
	if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return faviconEntry{Status: faviconUnavailable, RetryAfter: resp.Header.Get("Retry-After")}
	}
	if resp.StatusCode != http.StatusOK {
		return faviconEntry{Status: faviconMalformed}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFaviconBodyBytes+1))
	if err != nil {
		return faviconEntry{Status: faviconUnavailable}
	}
	if len(body) == 0 || int64(len(body)) > maxFaviconBodyBytes {
		return faviconEntry{Status: faviconMalformed}
	}
	contentType, ok := validatedImageContentType(resp.Header.Get("Content-Type"), body)
	if !ok {
		return faviconEntry{Status: faviconMalformed}
	}
	entry := faviconEntry{Body: body, ContentType: contentType, Status: faviconFound}
	return entry
}

func validatedImageContentType(header string, body []byte) (string, bool) {
	detected := http.DetectContentType(body)
	if strings.HasPrefix(detected, "image/") {
		return detected, true
	}

	declared, _, err := mime.ParseMediaType(header)
	if err != nil || !strings.HasPrefix(declared, "image/") {
		return "", false
	}
	if detected == "application/octet-stream" {
		return declared, true
	}
	if declared == "image/svg+xml" {
		trimmed := bytes.TrimSpace(body)
		if bytes.HasPrefix(trimmed, []byte("<svg")) ||
			(bytes.HasPrefix(trimmed, []byte("<?xml")) && bytes.Contains(trimmed, []byte("<svg"))) {
			return declared, true
		}
	}
	return "", false
}

func sanitizeRetryAfter(value string, now time.Time) string {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds <= uint64(maxRetryAfter/time.Second) {
			return strconv.FormatUint(seconds, 10)
		}
		return ""
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return ""
	}
	delay := retryAt.Sub(now)
	if delay < 0 || delay > maxRetryAfter {
		return ""
	}
	return retryAt.UTC().Format(http.TimeFormat)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		log.Printf("failed to encode response body: %v", err)
	}
}
