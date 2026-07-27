package main

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	listenPort int
	backend    string
	action     int // HTTP status code to return on block
	logLevel   slog.Level

	// Filter thresholds
	maxMentions     int
	maxContentRatio float64 // mentions / (non-mention chars) ratio

	blockKeywords []string
	blockDomains  []string

	// Server timeouts
	readTimeout     time.Duration
	writeTimeout    time.Duration
	idleTimeout     time.Duration
	shutdownTimeout time.Duration
}

func loadConfig() config {
	cfg := config{
		listenPort:      3000,
		action:          403,
		logLevel:        slog.LevelInfo,
		maxMentions:     4,
		maxContentRatio: 0.9,
		blockKeywords:   []string{},
		blockDomains:    []string{},
		readTimeout:     10 * time.Second,
		writeTimeout:    30 * time.Second,
		idleTimeout:     60 * time.Second,
		shutdownTimeout: 30 * time.Second,
	}

	if v := os.Getenv("LISTEN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.listenPort = p
		}
	}

	cfg.backend = os.Getenv("BACKEND")

	if v := os.Getenv("ACTION"); v != "" {
		if code, err := strconv.Atoi(v); err == nil && code >= 200 && code < 600 {
			cfg.action = code
		}
	}

	if v := os.Getenv("LOG_LEVEL"); v == "debug" {
		cfg.logLevel = slog.LevelDebug
	}

	if v := os.Getenv("MAX_MENTIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.maxMentions = n
		}
	}

	if v := os.Getenv("MAX_CONTENT_RATIO"); v != "" {
		if r, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.maxContentRatio = r
		}
	}

	if v := os.Getenv("BLOCK_KEYWORDS"); v != "" {
		cfg.blockKeywords = splitAndClean(v)
	}

	if v := os.Getenv("BLOCK_DOMAINS"); v != "" {
		cfg.blockDomains = splitAndClean(v)
	}

	if v := os.Getenv("READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.readTimeout = d
		}
	}

	if v := os.Getenv("WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.writeTimeout = d
		}
	}

	if v := os.Getenv("IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.idleTimeout = d
		}
	}

	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.shutdownTimeout = d
		}
	}

	return cfg
}

func splitAndClean(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
