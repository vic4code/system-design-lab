package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "beatstream_http_requests_total",
		Help: "Total HTTP requests by method, path, and status code.",
	}, []string{"method", "path", "status"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "beatstream_http_request_duration_seconds",
		Help:    "HTTP request latency distribution.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
	}, []string{"method", "path"})

	CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "beatstream_cache_hits_total",
		Help: "Redis cache hits by cache name (track, search).",
	}, []string{"cache"})

	CacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "beatstream_cache_misses_total",
		Help: "Redis cache misses by cache name (track, search).",
	}, []string{"cache"})

	RateLimitDecisions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "beatstream_rate_limit_decisions_total",
		Help: "Rate limit decisions for the stream endpoint.",
	}, []string{"decision"}) // allowed, allowed_auth, allowed_redis_error, denied
)
