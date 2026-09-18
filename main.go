package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func isBodyTooLarge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "request body too large")
}

func main() {
	cfg := loadConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.logLevel}))

	chain := buildFilterChain(cfg)

	if cfg.backend == "" {
		logger.Error("BACKEND environment variable is required but not set")
		os.Exit(1)
	}
	if cfg.mentionTarget != targetAlways && cfg.localDomain == "" {
		logger.Warn("MENTION_FILTER_TARGET is set but LOCAL_DOMAIN is empty; falling back to 'always' behavior")
	}
	backendURL, err := url.Parse(cfg.backend)
	if err != nil || !backendURL.IsAbs() || backendURL.Host == "" {
		logger.Error("invalid backend URL: must be an absolute URL with a host", "url", cfg.backend, "err", err)
		os.Exit(1)
	}
	// Only plain http backends on the trusted internal network are supported.
	// TLS to the backend is terminated by the outer reverse proxy, so https
	// here would hide the backend behind an opaque tunnel.
	if backendURL.Scheme != "http" {
		logger.Error("invalid backend URL scheme: only http is supported, https is not allowed", "url", cfg.backend)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	defaultDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalHost := req.Host
		defaultDirector(req)
		req.Host = originalHost
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Warn("proxy error", "err", err)
		trackError()
		w.WriteHeader(http.StatusBadGateway)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("GET /metrics", metricsHandler)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		trackRequest()
		if r.Method != http.MethodPost || r.Body == nil {
			trackProxied()
			proxy.ServeHTTP(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, cfg.maxBodyBytes))
		r.Body.Close()
		if err != nil {
			// Fail closed — never forward a truncated/unreadable body.
			logger.Warn("failed to read body", "err", err, "path", r.URL.Path)
			trackError()
			if isBodyTooLarge(err) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		reason, blocked, info := chain.CheckVerbose(bodyBytes, r)
		contentMentions := countMentions(info.Content)
		contentPreview := info.Content
		if len([]rune(contentPreview)) > 80 {
			contentPreview = string([]rune(contentPreview)[:80])
		}
		if blocked {
			logger.Debug("blocked.body", "raw", string(bodyBytes))
			logger.Info("blocked",
				"method", r.Method,
				"path", r.URL.Path,
				"body_len", len(bodyBytes),
				"reason", reason,
				"actor", info.Actor,
				"type", info.ActType,
				"content_type", r.Header.Get("Content-Type"),
				"content_mentions", contentMentions,
				"ap_mentions", info.APMentions,
				"in_reply_to", info.InReplyTo,
				"target_mode", cfg.mentionTarget,
				"content", contentPreview,
			)
			trackBlocked()
			w.WriteHeader(cfg.action)
			return
		}

		logger.Debug("passed.body", "raw", string(bodyBytes))
		logger.Info("passed",
			"method", r.Method,
			"path", r.URL.Path,
			"body_len", len(bodyBytes),
			"actor", info.Actor,
			"type", info.ActType,
			"content_type", r.Header.Get("Content-Type"),
			"content_mentions", contentMentions,
			"ap_mentions", info.APMentions,
			"in_reply_to", info.InReplyTo,
			"target_mode", cfg.mentionTarget,
			"content", contentPreview,
		)
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
		trackProxied()
		proxy.ServeHTTP(w, r)
	})

	handler := metricsMiddleware(mux)

	addr := fmt.Sprintf(":%d", cfg.listenPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  cfg.readTimeout,
		WriteTimeout: cfg.writeTimeout,
		IdleTimeout:  cfg.idleTimeout,
	}

	logger.Info("inbox-guard starting", "version", Version, "addr", addr, "backend", cfg.backend)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	sig := <-quit
	logger.Info("shutdown signal received", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "err", err)
		os.Exit(1)
	}
	logger.Info("server stopped gracefully")
}
