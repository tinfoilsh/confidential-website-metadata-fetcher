package main

import (
	"encoding/json"
	"errors"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
)

type metadataRequest struct {
	URL string `json:"url"`
}

type metadataResponse struct {
	URL    string  `json:"url"`
	Image  *string `json:"image"`
	Cached bool    `json:"cached"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Server wires the fetcher, cache, and HTTP handlers together so main.go can
// stand the service up with one call.
type Server struct {
	fetcher *fetch.Fetcher
	cache   *cache.LRU[fetch.Result]
}

func NewServer(fetcher *fetch.Fetcher, cache *cache.LRU[fetch.Result]) *Server {
	return &Server{fetcher: fetcher, cache: cache}
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
		writeJSON(w, http.StatusOK, metadataResponse{
			URL:    cached.URL,
			Image:  cached.Image,
			Cached: true,
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
		URL:    result.URL,
		Image:  result.Image,
		Cached: false,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		log.WithError(err).Debug("failed to encode response body")
	}
}
