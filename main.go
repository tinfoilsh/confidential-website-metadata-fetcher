package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/config"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/contextdev"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
)

const writeTimeoutGrace = 5 * time.Second

func main() {
	cfg := config.Load()
	if cfg.ContextDevAPIKey == "" {
		log.Fatal("CONTEXT_DEV_API_KEY is required")
	}
	contextDevClient := contextdev.NewClient(cfg.ContextDevAPIKey, cfg.FetchTimeout)
	fetcher := fetch.NewFetcher(contextDevClient, cfg.MaxBodyBytes)
	faviconClient := &http.Client{
		Timeout: cfg.FetchTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	server := NewServer(fetcher, faviconClient)

	mux := http.NewServeMux()
	server.Routes(mux)

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 2*cfg.FetchTimeout + writeTimeoutGrace,
		IdleTimeout:  90 * time.Second,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("metadata-fetch listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-sigChan
	log.Print("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
