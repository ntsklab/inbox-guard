package main

import (
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mention filter target modes.
const (
	targetAlways               = "always"
	targetMentioned            = "mentioned"
	targetInReplyTo            = "in_reply_to"
	targetMentionedOrInReplyTo = "mentioned_or_in_reply_to"
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

	// Mention filter targeting
	localDomain   string // our own instance domain
	mentionTarget string // always | mentioned | in_reply_to | mentioned_or_in_reply_to

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

	cfg.localDomain = normalizeDomain(os.Getenv("LOCAL_DOMAIN"))

	cfg.mentionTarget = targetAlways
	if v := os.Getenv("MENTION_FILTER_TARGET"); v != "" {
		switch v {
		case targetMentioned, targetInReplyTo, targetMentionedOrInReplyTo:
			cfg.mentionTarget = v
		}
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

// normalizeDomain accepts a bare domain ("instance.example") or a URL
// ("https://instance.example/") and returns the lowercase host.
func normalizeDomain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	s = strings.TrimSuffix(s, "/")
	if i := strings.LastIndex(s, ":"); i > 0 && !strings.Contains(s[i:], "/") {
		s = s[:i]
	}
	return strings.ToLower(s)
}
