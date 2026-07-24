package main

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type config struct {
	listenPort int
	backend    string
	action     string // "soft" or "block"
	logLevel   slog.Level

	// Filter thresholds
	maxMentions     int
	maxContentRatio float64 // mentions / (non-mention chars) ratio

	blockKeywords []string
	blockDomains  []string
}

func loadConfig() config {
	cfg := config{
		listenPort:      3000,
		action:          "soft",
		logLevel:        slog.LevelInfo,
		maxMentions:     4,
		maxContentRatio: 0.9,
		blockKeywords:   []string{},
		blockDomains:    []string{},
	}

	if v := os.Getenv("LISTEN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.listenPort = p
		}
	}

	cfg.backend = os.Getenv("BACKEND")

	if v := os.Getenv("ACTION"); v == "block" {
		cfg.action = "block"
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
