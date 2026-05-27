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

// LoginRateLimit protects auth endpoints against brute-force attacks.
// Strategy: sliding window counter in Redis.
//   - Key:    ratelimit:login:<ip>
//   - Window: 15 minutes
//   - Limit:  5 attempts per window
//   - On exceeded: 429 with Retry-After header
//   - On Redis error: fail open (don't block legitimate users)
//
// AWS mapping: this logic is replicated at the ALB level via AWS WAF
// (rate-based rule: 100 requests / 5 min per IP on /v1/auth/*).
// The application-level limit is the last line of defence.
func LoginRateLimit(rdb *redis.Client) gin.HandlerFunc {
	const (
		maxAttempts = 5
		windowSecs  = 15 * 60 // 15 minutes
	)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("ratelimit:login:%s", ip)
		ctx := context.Background()

		// Atomic increment + set expiry on first attempt
		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			// Redis unavailable — fail open
			c.Next()
			return
		}
		// Set expiry only on the first increment to implement sliding window
		if count == 1 {
			rdb.Expire(ctx, key, time.Duration(windowSecs)*time.Second)
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", maxAttempts))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, maxAttempts-int(count))))

		if count > maxAttempts {
			ttl, _ := rdb.TTL(ctx, key).Result()
			c.Header("Retry-After", fmt.Sprintf("%d", int(ttl.Seconds())))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "too many login attempts — try again later",
				"retry_after": int(ttl.Seconds()),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

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
