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
	"syscall"
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
		if r.Method != http.MethodPost || r.Body == nil {
			trackProxied()
			proxy.ServeHTTP(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			logger.Warn("failed to read body", "err", err)
			trackError()
			proxy.ServeHTTP(w, r)
			return
		}

		reason, blocked := chain.Check(bodyBytes, r)
		if blocked {
			_, actor := getContent(bodyBytes)
			logger.Info("blocked", "reason", reason, "actor", actor)
			trackBlocked()
			if cfg.action == "soft" {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusForbidden)
			}
			return
		}

		_, actor := getContent(bodyBytes)
		logger.Info("passed", "actor", actor)
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

	logger.Info("inbox-guard starting", "addr", addr, "backend", cfg.backend)

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
