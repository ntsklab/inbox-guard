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
		globalMetrics.requestsTotal.Add(1)
		next.ServeHTTP(w, r)
	})
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

	uptime := time.Since(globalMetrics.uptime).Seconds()

	var b []byte
	b = appendMetric(b, "inbox_guard_uptime_seconds", "gauge", uptime)
	b = appendMetric(b, "inbox_guard_requests_total", "counter", float64(globalMetrics.requestsTotal.Load()))
	b = appendMetric(b, "inbox_guard_requests_blocked_total", "counter", float64(globalMetrics.requestsBlocked.Load()))
	b = appendMetric(b, "inbox_guard_requests_proxied_total", "counter", float64(globalMetrics.requestsProxied.Load()))
	b = appendMetric(b, "inbox_guard_requests_errored_total", "counter", float64(globalMetrics.requestsErrored.Load()))

	w.Write(b)
}

func appendMetric(buf []byte, name, mtype string, value float64) []byte {
	return append(buf,
		fmt.Sprintf("# TYPE %s %s\n%s %g\n", name, mtype, name, value)...)
}
