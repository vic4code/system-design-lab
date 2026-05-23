// Standalone token bucket rate limiter using Redis.
// Run: go run main.go
// Requires Redis on localhost:6379.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucketScript is an atomic Lua script that implements a token bucket.
// It is atomic because Redis executes Lua scripts without interruption.
// A plain GET+SET would allow two goroutines to both read the same token
// count and both pass when only one should.
const tokenBucketScript = `
local key         = KEYS[1]
local max_tokens  = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now         = tonumber(ARGV[3])
local cost        = tonumber(ARGV[4])

local data        = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens      = tonumber(data[1]) or max_tokens
local last_refill = tonumber(data[2]) or now

-- Refill tokens based on elapsed time
local elapsed = now - last_refill
tokens = math.min(max_tokens, tokens + elapsed * refill_rate)

if tokens < cost then
  -- Denied: return {0, seconds_until_retry}
  return {0, math.ceil((cost - tokens) / refill_rate)}
end

tokens = tokens - cost
redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, math.ceil(max_tokens / refill_rate) * 2)
return {1, 0}
`

type RateLimiter struct {
	rdb    *redis.Client
	script *redis.Script
}

func NewRateLimiter(addr string) *RateLimiter {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	return &RateLimiter{
		rdb:    rdb,
		script: redis.NewScript(tokenBucketScript),
	}
}

type Result struct {
	Allowed    bool
	RetryAfter time.Duration
}

// Allow checks whether the given key is within rate limits.
// maxTokens: bucket capacity; refillRate: tokens/second; cost: tokens this request consumes.
func (r *RateLimiter) Allow(ctx context.Context, key string, maxTokens, refillRate, cost int) (Result, error) {
	now := float64(time.Now().UnixNano()) / 1e9

	res, err := r.script.Run(ctx, r.rdb,
		[]string{key},
		maxTokens, refillRate, now, cost,
	).Int64Slice()
	if err != nil {
		return Result{}, err
	}

	if res[0] == 1 {
		return Result{Allowed: true}, nil
	}
	return Result{
		Allowed:    false,
		RetryAfter: time.Duration(res[1]) * time.Second,
	}, nil
}

func main() {
	rl := NewRateLimiter("localhost:6379")
	ctx := context.Background()

	const (
		userKey    = "rate_limit:user:demo:stream"
		maxTokens  = 5
		refillRate = 1 // 1 token per second
		cost       = 1
	)

	fmt.Printf("Token bucket demo: capacity=%d, refill=%d/s\n\n", maxTokens, refillRate)

	for i := 1; i <= 10; i++ {
		res, err := rl.Allow(ctx, userKey, maxTokens, refillRate, cost)
		if err != nil {
			fmt.Printf("request %2d: ERROR %v\n", i, err)
			continue
		}
		if res.Allowed {
			fmt.Printf("request %2d: ALLOWED\n", i)
		} else {
			fmt.Printf("request %2d: DENIED  (retry after %s)\n", i, res.RetryAfter)
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("\nWaiting 3 seconds for token refill...")
	time.Sleep(3 * time.Second)

	for i := 11; i <= 15; i++ {
		res, _ := rl.Allow(ctx, userKey, maxTokens, refillRate, cost)
		if res.Allowed {
			fmt.Printf("request %2d: ALLOWED (after refill)\n", i)
		} else {
			fmt.Printf("request %2d: DENIED\n", i)
		}
	}
}
