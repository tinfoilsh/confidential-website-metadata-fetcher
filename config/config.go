package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr      string
	FetchTimeout    time.Duration
	MaxRedirects    int
	MaxBodyBytes    int64
	MaxConcurrent   int
	UserAgent       string
	CacheMaxEntries int
	CacheTTL        time.Duration
}

func Load() *Config {
	return &Config{
		ListenAddr:      getEnv("LISTEN_ADDR", ":8089"),
		FetchTimeout:    getEnvDuration("FETCH_TIMEOUT", 5*time.Second),
		MaxRedirects:    getEnvInt("MAX_REDIRECTS", 5),
		MaxBodyBytes:    getEnvInt64("MAX_BODY_BYTES", 5*1024*1024),
		MaxConcurrent:   getEnvInt("MAX_CONCURRENT", 32),
		UserAgent:       getEnv("USER_AGENT", "Mozilla/5.0 (compatible; MetadataFetchBot/1.0; +https://github.com)"),
		CacheMaxEntries: getEnvInt("CACHE_MAX_ENTRIES", 5000),
		CacheTTL:        getEnvDuration("CACHE_TTL", 24*time.Hour),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
