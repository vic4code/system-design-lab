package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vic4code/system-design-lab/beatstream/internal/metrics"
)

// PrometheusMetrics records request count and latency for every handled route.
func PrometheusMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath() // e.g. /v1/tracks/:id — collapses dynamic segments
		if path == "" {
			path = "unknown"
		}

		elapsed := time.Since(start).Seconds()
		metrics.RequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		metrics.RequestDuration.WithLabelValues(c.Request.Method, path).Observe(elapsed)
	}
}
