package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
)

const (
	maxRequestBodyBytes int64 = 64 * 1024
	maxFaviconBodyBytes int64 = 256 * 1024
	faviconURLTemplate        = "https://icons.duckduckgo.com/ip3/%s.ico"
)

type metadataRequest struct {
	URL string `json:"url"`
}

type faviconResponse struct {
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

// Server wires the fetcher, cache, and HTTP handlers together so main.go can
// stand the service up with one call.
type Server struct {
	fetcher       *fetch.Fetcher
	cache         *cache.LRU[fetch.Result]
	faviconClient httpDoer
	faviconCache  *cache.LRU[faviconEntry]
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewServer(
	fetcher *fetch.Fetcher,
	faviconClient httpDoer,
	cacheMaxEntries int,
	cacheTTL time.Duration,
) *Server {
	return &Server{
		fetcher:       fetcher,
		cache:         cache.New[fetch.Result](cacheMaxEntries, cacheTTL),
		faviconClient: faviconClient,
		faviconCache:  cache.New[faviconEntry](cacheMaxEntries, cacheTTL),
	}
}

type faviconEntry struct {
	Body        []byte
	ContentType string
}

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

	if cached, ok := s.cache.Get(targetURL); ok {
		writeJSON(w, http.StatusOK, metadataResponse{
			Result: cached,
			Cached: true,
		})
		return
	}

	result, err := s.fetcher.Fetch(ctx, targetURL)
	if err != nil {
		log.Print("metadata fetch failed")
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch metadata"})
		return
	}

	s.cache.Set(targetURL, *result)
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
	if len(favicon.Body) == 0 {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch favicon"})
		return
	}
	writeJSON(w, http.StatusOK, faviconResponse{
		FaviconBytes:       favicon.Body,
		FaviconContentType: favicon.ContentType,
	})
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
		return faviconEntry{}
	}
	host := parsed.Hostname()
	if host == "" {
		return faviconEntry{}
	}

	if entry, ok := s.faviconCache.Get(host); ok {
		return entry
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf(faviconURLTemplate, url.PathEscape(host)),
		nil,
	)
	if err != nil {
		return faviconEntry{}
	}
	resp, err := s.faviconClient.Do(req)
	if err != nil {
		return faviconEntry{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return faviconEntry{}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFaviconBodyBytes+1))
	if err != nil || int64(len(body)) > maxFaviconBodyBytes {
		return faviconEntry{}
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(contentType, "image/") {
		contentType = http.DetectContentType(body)
		if !strings.HasPrefix(contentType, "image/") {
			return faviconEntry{}
		}
	}
	entry := faviconEntry{Body: body, ContentType: contentType}
	s.faviconCache.Set(host, entry)
	return entry
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		log.Printf("failed to encode response body: %v", err)
	}
}
