package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr   string
	FetchTimeout time.Duration
	MaxBodyBytes int64
	ZyteAPIKey   string
}

func Load() *Config {
	return &Config{
		ListenAddr:   getEnv("LISTEN_ADDR", ":8089"),
		FetchTimeout: getEnvDuration("FETCH_TIMEOUT", 15*time.Second),
		MaxBodyBytes: getEnvInt64("MAX_BODY_BYTES", 5*1024*1024),
		ZyteAPIKey:   os.Getenv("ZYTE_API_KEY"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
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
