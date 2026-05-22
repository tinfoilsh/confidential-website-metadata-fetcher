package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/favicon"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
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
	URL                string  `json:"url"`
	Title              *string `json:"title"`
	Description        *string `json:"description"`
	SiteName           *string `json:"site_name"`
	Image              *string `json:"image"`
	FaviconBytes       []byte  `json:"favicon_bytes,omitempty"`
	FaviconContentType *string `json:"favicon_content_type,omitempty"`
	Cached             bool    `json:"cached"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Server wires the fetcher, cache, and HTTP handlers together so main.go can
// stand the service up with one call.
type Server struct {
	fetcher        *fetch.Fetcher
	cache          *cache.LRU[fetch.Result]
	faviconFetcher *favicon.Fetcher
}

func NewServer(
	fetcher *fetch.Fetcher,
	cache *cache.LRU[fetch.Result],
	faviconFetcher *favicon.Fetcher,
) *Server {
	return &Server{fetcher: fetcher, cache: cache, faviconFetcher: faviconFetcher}
}

// Routes registers the service endpoints on the given mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/metadata", s.handleMetadata)
	mux.HandleFunc("/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	var req metadataRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	cacheKey := cache.NormalizeURL(req.URL)
	if cached, ok := s.cache.Get(cacheKey); ok {
		bytes, contentType := s.fetchFavicon(r, cached.URL)
		writeJSON(w, http.StatusOK, metadataResponse{
			URL:                cached.URL,
			Title:              cached.Title,
			Description:        cached.Description,
			SiteName:           cached.SiteName,
			Image:              cached.Image,
			FaviconBytes:       bytes,
			FaviconContentType: contentType,
			Cached:             true,
		})
		return
	}

	result, err := s.fetcher.Fetch(r.Context(), req.URL)
	if err != nil {
		if fetch.IsClientError(err) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		log.WithFields(log.Fields{"err": err.Error()}).Warn("metadata fetch failed")
		// Favicon only needs the hostname, so a paywalled, timed-out,
		// or bot-blocked page should still surface its icon. Try the
		// favicon lookup directly and return a partial response when
		// it succeeds; only fall through to 502 if both lookups fail.
		bytes, contentType := s.fetchFavicon(r, req.URL)
		if len(bytes) > 0 {
			writeJSON(w, http.StatusOK, metadataResponse{
				URL:                req.URL,
				FaviconBytes:       bytes,
				FaviconContentType: contentType,
				Cached:             false,
			})
			return
		}
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch metadata"})
		return
	}

	s.cache.Set(cacheKey, *result)
	bytes, contentType := s.fetchFavicon(r, result.URL)
	writeJSON(w, http.StatusOK, metadataResponse{
		URL:                result.URL,
		Title:              result.Title,
		Description:        result.Description,
		SiteName:           result.SiteName,
		Image:              result.Image,
		FaviconBytes:       bytes,
		FaviconContentType: contentType,
		Cached:             false,
	})
}

// fetchFavicon proxies the favicon for the resolved page URL through the
// enclave so clients never reach an external icon host directly. A
// failure is non-fatal and yields empty bytes; the client falls back to a
// placeholder icon.
func (s *Server) fetchFavicon(r *http.Request, pageURL string) ([]byte, *string) {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return nil, nil
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, nil
	}

	entry, _, err := s.faviconFetcher.Fetch(r.Context(), host)
	if err != nil {
		if !errors.Is(err, favicon.ErrInvalidHost) {
			log.WithFields(log.Fields{"err": err.Error(), "host": host}).Debug("favicon fetch failed")
		}
		return nil, nil
	}
	contentType := entry.ContentType
	return entry.Body, &contentType
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		log.WithError(err).Debug("failed to encode response body")
	}
}
