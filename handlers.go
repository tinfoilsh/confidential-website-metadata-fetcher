package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/favicon"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
)

type metadataRequest struct {
	URL string `json:"url"`
}

type metadataResponse struct {
	URL         string  `json:"url"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	SiteName    *string `json:"site_name"`
	Image       *string `json:"image"`
	Favicon     *string `json:"favicon"`
	Cached      bool    `json:"cached"`
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
	mux.HandleFunc("/favicon", s.handleFavicon)
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
		writeJSON(w, http.StatusOK, metadataResponse{
			URL:         cached.URL,
			Title:       cached.Title,
			Description: cached.Description,
			SiteName:    cached.SiteName,
			Image:       cached.Image,
			Favicon:     cached.Favicon,
			Cached:      true,
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
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch metadata"})
		return
	}

	s.cache.Set(cacheKey, *result)
	writeJSON(w, http.StatusOK, metadataResponse{
		URL:         result.URL,
		Title:       result.Title,
		Description: result.Description,
		SiteName:    result.SiteName,
		Image:       result.Image,
		Favicon:     result.Favicon,
		Cached:      false,
	})
}

// handleFavicon proxies a favicon lookup to the upstream icon service.
// The caller supplies only a hostname so there is no way to coerce the
// enclave into reaching arbitrary endpoints through this path.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "host query parameter is required"})
		return
	}

	entry, cached, err := s.faviconFetcher.Fetch(r.Context(), host)
	if err != nil {
		if errors.Is(err, favicon.ErrInvalidHost) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		log.WithFields(log.Fields{"err": err.Error(), "host": host}).Warn("favicon fetch failed")
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch favicon"})
		return
	}

	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(entry.Body)))
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	if cached {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(entry.Body); err != nil {
		log.WithError(err).Debug("failed to write favicon body")
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		log.WithError(err).Debug("failed to encode response body")
	}
}
