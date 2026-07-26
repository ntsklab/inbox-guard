package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type metrics struct {
	requestsTotal   atomic.Int64
	requestsBlocked atomic.Int64
	requestsProxied atomic.Int64
	requestsErrored atomic.Int64
	uptime          time.Time
}

var globalMetrics = metrics{
	uptime: time.Now(),
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func trackRequest() {
	globalMetrics.requestsTotal.Add(1)
}

func trackBlocked() {
	globalMetrics.requestsBlocked.Add(1)
}

func trackProxied() {
	globalMetrics.requestsProxied.Add(1)
}

func trackError() {
	globalMetrics.requestsErrored.Add(1)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var b []byte

	// HELP / TYPE lines (one per metric name)
	b = append(b, "# HELP inbox_guard_uptime_seconds Server uptime in seconds.\n# TYPE inbox_guard_uptime_seconds gauge\n"...)
	b = append(b, "# HELP inbox_guard_requests_total Total ActivityPub requests processed (excludes health/metrics).\n# TYPE inbox_guard_requests_total counter\n"...)
	b = append(b, "# HELP inbox_guard_requests_blocked_total Requests blocked by filters.\n# TYPE inbox_guard_requests_blocked_total counter\n"...)
	b = append(b, "# HELP inbox_guard_requests_proxied_total Requests proxied to backend.\n# TYPE inbox_guard_requests_proxied_total counter\n"...)
	b = append(b, "# HELP inbox_guard_requests_errored_total Requests that resulted in errors.\n# TYPE inbox_guard_requests_errored_total counter\n"...)

	// Metric values
	uptime := time.Since(globalMetrics.uptime).Seconds()
	b = fmt.Appendf(b, "inbox_guard_uptime_seconds %g\n", uptime)
	b = fmt.Appendf(b, "inbox_guard_requests_total %d\n", globalMetrics.requestsTotal.Load())
	b = fmt.Appendf(b, "inbox_guard_requests_blocked_total %d\n", globalMetrics.requestsBlocked.Load())
	b = fmt.Appendf(b, "inbox_guard_requests_proxied_total %d\n", globalMetrics.requestsProxied.Load())
	b = fmt.Appendf(b, "inbox_guard_requests_errored_total %d\n", globalMetrics.requestsErrored.Load())

	w.Write(b)
}
