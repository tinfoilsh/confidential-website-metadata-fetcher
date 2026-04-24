package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/tinfoilsh/confidential-website-metadata-fetcher/cache"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/config"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/favicon"
	"github.com/tinfoilsh/confidential-website-metadata-fetcher/fetch"
)

var verbose = flag.Bool("v", false, "enable verbose logging")

func main() {
	flag.Parse()
	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	cfg := config.Load()
	fetcher := fetch.NewFetcher(cfg)
	resultCache := cache.New[fetch.Result](cfg.CacheMaxEntries, cfg.CacheTTL)
	faviconFetcher := favicon.NewFetcher(
		cfg.FetchTimeout,
		cfg.CacheMaxEntries,
		cfg.CacheTTL,
	)
	server := NewServer(fetcher, resultCache, faviconFetcher)

	mux := http.NewServeMux()
	server.Routes(mux)

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Infof("metadata-fetch listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-sigChan
	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.WithError(err).Warn("graceful shutdown failed")
	}
}
