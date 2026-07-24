package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

func main() {
	cfg := loadConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.logLevel}))

	chain := buildFilterChain(cfg)

	if cfg.backend == "" {
		logger.Error("BACKEND environment variable is required but not set")
		os.Exit(1)
	}
	backendURL, err := url.Parse(cfg.backend)
	if err != nil {
		logger.Error("invalid backend URL", "url", cfg.backend, "err", err)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	mux := http.NewServeMux()

	// health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// all other routes = inbox handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil {
			proxy.ServeHTTP(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			logger.Warn("failed to read body", "err", err)
			proxy.ServeHTTP(w, r)
			return
		}

		// Run filters
		reason, blocked := chain.Check(bodyBytes, r)
		if blocked {
			logger.Info("blocked", "reason", reason)
			if cfg.action == "soft" {
				// Return 200 so the sender thinks it succeeded (prevents retry)
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusForbidden)
			}
			return
		}

		// Pass to backend
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
		proxy.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", cfg.listenPort)
	logger.Info("inbox-guard starting", "addr", addr, "backend", cfg.backend)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}
