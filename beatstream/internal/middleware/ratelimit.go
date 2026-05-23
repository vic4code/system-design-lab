package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vic4code/system-design-lab/beatstream/internal/metrics"
)

// tokenBucketScript is a Redis Lua script implementing the token bucket algorithm.
// It is loaded once and run atomically, preventing race conditions across API instances.
//
// KEYS[1]: bucket key (per-IP)
// ARGV[1]: max tokens (bucket capacity)
// ARGV[2]: refill rate in tokens/second
// ARGV[3]: current Unix time as float (seconds)
// ARGV[4]: tokens to consume (always 1)
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local max_tokens   = tonumber(ARGV[1])
local refill_rate  = tonumber(ARGV[2])
local now          = tonumber(ARGV[3])
local requested    = tonumber(ARGV[4])

local data        = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens      = tonumber(data[1]) or max_tokens
local last_refill = tonumber(data[2]) or now

local elapsed   = math.max(0, now - last_refill)
local new_tokens = math.min(max_tokens, tokens + elapsed * refill_rate)

if new_tokens >= requested then
    redis.call('HMSET', key, 'tokens', new_tokens - requested, 'last_refill', now)
    redis.call('EXPIRE', key, 7200)
    return 1
else
    redis.call('HMSET', key, 'tokens', new_tokens, 'last_refill', now)
    redis.call('EXPIRE', key, 7200)
    return 0
end
`)

// StreamRateLimit applies a token bucket rate limit to the stream endpoint.
// Authenticated requests (X-User-ID header present) are exempt.
// On Redis failure the middleware fails open so the service stays available.
func StreamRateLimit(rdb *redis.Client, maxPerHour int) gin.HandlerFunc {
	refillRate := float64(maxPerHour) / 3600.0

	return func(c *gin.Context) {
		if c.GetHeader("X-User-ID") != "" {
			metrics.RateLimitDecisions.WithLabelValues("allowed_auth").Inc()
			c.Next()
			return
		}

		ip := c.ClientIP()
		key := fmt.Sprintf("ratelimit:stream:%s", ip)
		now := float64(time.Now().UnixNano()) / 1e9

		result, err := tokenBucketScript.Run(
			context.Background(),
			rdb,
			[]string{key},
			float64(maxPerHour),
			refillRate,
			now,
			1,
		).Int()

		if err != nil {
			// Redis unavailable — fail open to preserve availability.
			metrics.RateLimitDecisions.WithLabelValues("allowed_redis_error").Inc()
			c.Next()
			return
		}

		if result == 0 {
			metrics.RateLimitDecisions.WithLabelValues("denied").Inc()
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d/hour", maxPerHour))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded: 100 streams per hour for unauthenticated requests"})
			c.Abort()
			return
		}

		metrics.RateLimitDecisions.WithLabelValues("allowed").Inc()
		c.Next()
	}
}
