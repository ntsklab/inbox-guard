package main

import (
	"fmt"
	"log/slog"
	"math"
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
	maxBodyBytes    int64

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

func loadConfig() (config, error) {
	cfg := config{
		listenPort:      3000,
		action:          403,
		logLevel:        slog.LevelInfo,
		maxMentions:     4,
		maxContentRatio: 0.9,
		maxBodyBytes:    1 << 20, // 1 MiB
		blockKeywords:   []string{},
		blockDomains:    []string{},
		readTimeout:     10 * time.Second,
		writeTimeout:    30 * time.Second,
		idleTimeout:     60 * time.Second,
		shutdownTimeout: 30 * time.Second,
	}

	if v := os.Getenv("LISTEN_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid LISTEN_PORT %q: must be an integer", v)
		}
		cfg.listenPort = p
	}
	if cfg.listenPort < 1 || cfg.listenPort > 65535 {
		return config{}, fmt.Errorf("invalid LISTEN_PORT %d: must be 1-65535", cfg.listenPort)
	}

	cfg.backend = os.Getenv("BACKEND")

	if v := os.Getenv("ACTION"); v != "" {
		code, err := strconv.Atoi(v)
		if err != nil || code < 200 || code >= 600 {
			return config{}, fmt.Errorf("invalid ACTION %q: must be an integer 200-599", v)
		}
		cfg.action = code
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch v {
		case "info":
			cfg.logLevel = slog.LevelInfo
		case "debug":
			cfg.logLevel = slog.LevelDebug
		default:
			return config{}, fmt.Errorf("invalid LOG_LEVEL %q: must be info or debug", v)
		}
	}

	if v := os.Getenv("MAX_MENTIONS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid MAX_MENTIONS %q: must be an integer", v)
		}
		cfg.maxMentions = n
	}
	if cfg.maxMentions < 0 {
		return config{}, fmt.Errorf("invalid MAX_MENTIONS %d: must be >= 0", cfg.maxMentions)
	}

	if v := os.Getenv("MAX_CONTENT_RATIO"); v != "" {
		r, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return config{}, fmt.Errorf("invalid MAX_CONTENT_RATIO %q: must be a number", v)
		}
		cfg.maxContentRatio = r
	}
	if math.IsNaN(cfg.maxContentRatio) || math.IsInf(cfg.maxContentRatio, 0) ||
		cfg.maxContentRatio < 0 || cfg.maxContentRatio > 1 {
		return config{}, fmt.Errorf("invalid MAX_CONTENT_RATIO %v: must be 0.0-1.0", cfg.maxContentRatio)
	}

	if v := os.Getenv("MAX_BODY_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return config{}, fmt.Errorf("invalid MAX_BODY_BYTES %q: must be an integer", v)
		}
		cfg.maxBodyBytes = n
	}
	if cfg.maxBodyBytes <= 0 || cfg.maxBodyBytes > 32<<20 {
		return config{}, fmt.Errorf("invalid MAX_BODY_BYTES %d: must be 1-%d", cfg.maxBodyBytes, 32<<20)
	}

	if v := os.Getenv("BLOCK_KEYWORDS"); v != "" {
		cfg.blockKeywords = splitAndClean(v)
	}

	if v := os.Getenv("BLOCK_DOMAINS"); v != "" {
		for _, d := range splitAndClean(v) {
			if nd := normalizeDomain(d); nd != "" {
				cfg.blockDomains = append(cfg.blockDomains, nd)
			}
		}
	}

	cfg.localDomain = normalizeDomain(os.Getenv("LOCAL_DOMAIN"))

	cfg.mentionTarget = targetAlways
	if v := os.Getenv("MENTION_FILTER_TARGET"); v != "" {
		switch v {
		case targetAlways, targetMentioned, targetInReplyTo, targetMentionedOrInReplyTo:
			cfg.mentionTarget = v
		default:
			return config{}, fmt.Errorf("invalid MENTION_FILTER_TARGET %q: must be always, mentioned, in_reply_to or mentioned_or_in_reply_to", v)
		}
	}

	if v := os.Getenv("READ_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid READ_TIMEOUT %q: %v", v, err)
		}
		cfg.readTimeout = d
	}

	if v := os.Getenv("WRITE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid WRITE_TIMEOUT %q: %v", v, err)
		}
		cfg.writeTimeout = d
	}

	if v := os.Getenv("IDLE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid IDLE_TIMEOUT %q: %v", v, err)
		}
		cfg.idleTimeout = d
	}

	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT %q: %v", v, err)
		}
		cfg.shutdownTimeout = d
	}
	for _, tv := range []struct {
		name string
		d    time.Duration
	}{
		{"READ_TIMEOUT", cfg.readTimeout},
		{"WRITE_TIMEOUT", cfg.writeTimeout},
		{"IDLE_TIMEOUT", cfg.idleTimeout},
		{"SHUTDOWN_TIMEOUT", cfg.shutdownTimeout},
	} {
		if tv.d <= 0 {
			return config{}, fmt.Errorf("invalid %s %v: must be > 0", tv.name, tv.d)
		}
	}

	return cfg, nil
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
