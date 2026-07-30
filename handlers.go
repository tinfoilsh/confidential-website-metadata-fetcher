package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/zyte"
)

const (
	maxRequestBodyBytes int64 = 64 * 1024
	maxFaviconBodyBytes int64 = 256 * 1024
	faviconURLTemplate        = "https://icons.duckduckgo.com/ip3/%s.ico"
)

type metadataRequest struct {
	URL string `json:"url"`
}

// metadataResponse mirrors the JSON envelope returned to clients. Favicon
// bytes are base64-encoded inline so the caller never has to make a
// follow-up GET to an external host. FaviconContentType is whatever the
// upstream icon service declared (typically image/x-icon or image/png) so
// the client can build a data URL or Blob without sniffing.
type metadataResponse struct {
	fetch.Result
	FaviconBytes       []byte `json:"favicon_bytes,omitempty"`
	FaviconContentType string `json:"favicon_content_type,omitempty"`
	Cached             bool   `json:"cached"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Server wires the fetcher, cache, and HTTP handlers together so main.go can
// stand the service up with one call.
type Server struct {
	fetcher      *fetch.Fetcher
	cache        *cache.LRU[fetch.Result]
	upstream     zyte.Fetcher
	faviconCache *cache.LRU[faviconEntry]
}

func NewServer(
	fetcher *fetch.Fetcher,
	upstream zyte.Fetcher,
	cacheMaxEntries int,
	cacheTTL time.Duration,
) *Server {
	return &Server{
		fetcher:      fetcher,
		cache:        cache.New[fetch.Result](cacheMaxEntries, cacheTTL),
		upstream:     upstream,
		faviconCache: cache.New[faviconEntry](cacheMaxEntries, cacheTTL),
	}
}

type faviconEntry struct {
	Body        []byte
	ContentType string
}

// Routes registers the service endpoints on the given mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/metadata", s.handleMetadata)
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

	var req metadataRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	targetURL, err := fetch.NormalizeTargetURL(req.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	ctx := r.Context()

	if cached, ok := s.cache.Get(targetURL); ok {
		favicon := s.fetchFavicon(ctx, cached.URL)
		writeJSON(w, http.StatusOK, metadataResponse{
			Result:             cached,
			FaviconBytes:       favicon.Body,
			FaviconContentType: favicon.ContentType,
			Cached:             true,
		})
		return
	}

	result, err := s.fetcher.Fetch(ctx, targetURL)
	if err != nil {
		log.Print("metadata fetch failed")
		// Favicon only needs the hostname, so a paywalled, timed-out,
		// or bot-blocked page should still surface its icon. Try the
		// favicon lookup directly and return a partial response when
		// it succeeds; only fall through to 502 if both lookups fail.
		favicon := s.fetchFavicon(ctx, targetURL)
		if len(favicon.Body) > 0 {
			writeJSON(w, http.StatusOK, metadataResponse{
				Result:             fetch.Result{URL: targetURL},
				FaviconBytes:       favicon.Body,
				FaviconContentType: favicon.ContentType,
				Cached:             false,
			})
			return
		}
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch metadata"})
		return
	}

	s.cache.Set(targetURL, *result)
	favicon := s.fetchFavicon(ctx, result.URL)
	writeJSON(w, http.StatusOK, metadataResponse{
		Result:             *result,
		FaviconBytes:       favicon.Body,
		FaviconContentType: favicon.ContentType,
		Cached:             false,
	})
}

// fetchFavicon proxies the favicon through Zyte so clients never reach an
// external icon host directly. A failure is non-fatal and yields empty bytes.
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
	resp, err := s.upstream.Fetch(
		ctx,
		fmt.Sprintf(faviconURLTemplate, url.PathEscape(host)),
		maxFaviconBodyBytes,
	)
	if err != nil {
		return faviconEntry{}
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "image/x-icon"
	}
	entry := faviconEntry{Body: resp.Body, ContentType: contentType}
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
