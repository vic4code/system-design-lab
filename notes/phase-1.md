# Phase 1 — Load Balancing & Caching

## What we built

Put the system under real load with k6 (up to 500 VUs), added Redis caching, a token bucket rate limiter, Prometheus metrics, and a Grafana dashboard. nginx load balances across 3 API instances.

**Load test result:** 325,009 requests in 3m30s, p95 latency 11ms, cache hit rate ~99.997%.

---

## Key concepts

### Cache-aside (lazy loading)

```
read request → check Redis
                   ↓
              hit? → return cached JSON (2ms)
              miss? → query Postgres → store in Redis → return (15ms)
```

Implementation in [`internal/handler/tracks.go`](../beatstream/internal/handler/tracks.go):
- Cache key: `track:{uuid}`, TTL: 1 hour
- On miss: query DB, marshal to JSON, `SET` in Redis
- On Redis error: fall through to DB silently (fail-open)

**Why cache-aside and not write-through?**
Write-through updates the cache on every write. Cache-aside only populates on reads. For track metadata that rarely changes, cache-aside is simpler — you don't pay the cache write cost on uploads, and a cold start is fine (first read populates it).

**Why 1 hour TTL for tracks, 1 minute for search?**
Track metadata (title, artist) barely changes — 1 hour is safe. Search results reflect play counts and new tracks, so stale results are more noticeable — 1 minute balances freshness vs DB load.

### Token bucket rate limiter

The algorithm: each IP has a "bucket" that holds up to N tokens. Each request consumes 1 token. Tokens refill continuously at a fixed rate.

```
capacity = 100 tokens
refill   = 100 / 3600 = 0.0278 tokens/second

at t=0:    bucket = 100  → allow
at t=0.1:  bucket = 99   → allow
...
at t=100:  bucket = 0    → deny (429)
at t=3700: bucket = ~2.8 → allow again (refilled over time)
```

**Why a Redis Lua script?**
The rate limiter needs to: read the current token count, calculate elapsed time, add refilled tokens, subtract 1, write back. That's a read-modify-write — a classic race condition if two API instances do it simultaneously. A Lua script runs **atomically** on the Redis server, so no race condition regardless of how many API instances exist.

```lua
-- simplified
local tokens = redis.call('HGET', key, 'tokens') or max_tokens
local elapsed = now - last_refill
local new_tokens = math.min(max_tokens, tokens + elapsed * refill_rate)
if new_tokens >= 1 then
    redis.call('HMSET', key, 'tokens', new_tokens - 1, 'last_refill', now)
    return 1  -- allowed
end
return 0  -- denied
```

**Fail-open design:** if Redis is unavailable, the middleware lets the request through. Availability > rate limiting strictness.

**Authenticated users are exempt:** requests with `X-User-ID` header bypass the limiter entirely. Paid/logged-in users should never be blocked.

### Load balancing with nginx least_conn

```nginx
upstream beatstream_api {
    least_conn;
    server api-1:8080 max_fails=3 fail_timeout=30s;
    server api-2:8080 max_fails=3 fail_timeout=30s;
    server api-3:8080 max_fails=3 fail_timeout=30s;
}
```

`least_conn` routes each new request to the instance with the fewest active connections. Better than round-robin for requests with variable duration (e.g., stream redirects are fast, file uploads are slow).

**Health checks:** `max_fails=3 fail_timeout=30s` — if an instance fails 3 requests in 30 seconds, nginx stops sending traffic to it. When you `docker compose stop api-2`, the other two absorb the load with near-zero errors.

### Prometheus metrics

Four metric types we use:

| Metric | Type | What it measures |
|--------|------|-----------------|
| `beatstream_http_requests_total` | Counter | request count by method/path/status |
| `beatstream_http_request_duration_seconds` | Histogram | latency distribution (p95, p99) |
| `beatstream_cache_hits_total` | Counter | Redis hits by cache name |
| `beatstream_rate_limit_decisions_total` | Counter | allowed / denied decisions |

The API exposes `/metrics` on port 9090. Prometheus scrapes it every 15 seconds. Grafana queries Prometheus to draw graphs.

---

## Gotchas discovered

- **Grafana datasource UID mismatch:** the provisioned datasource YAML must explicitly set `uid: prometheus` to match the dashboard JSON. Without it, Grafana auto-generates a random UID and the panels show "No data".
- **Cache hit rate panel shows "No data" after load test ends:** `rate(...[1m])` returns 0 when there's no recent traffic. Dividing 0/0 = NaN. Fix: use `increase(...[5m])` with `clamp_min(..., 1)` as the denominator.
- **k6 error rate 20% with rate limiter:** not a bug. k6 counts 429 as an error. The rate limiter is working correctly — 500 VUs sharing the same IP will exhaust 100 tokens/hour almost immediately. In production each user has their own IP/token.

---

## Experiment results

**Kill one API instance mid-test:**
```bash
docker compose stop api-2
# k6 continues with ~0 additional errors
# nginx detects the failure via max_fails and stops routing to api-2
```

**Kill Redis:**
```bash
docker compose stop redis
curl http://localhost/v1/tracks/:id  # still returns 200 — falls back to DB
# rate limiter also fails open — all stream requests allowed
```

---

## Interview talking points

**"Your /stream endpoint is getting hammered. Walk me through your caching strategy."**
> I use cache-aside on the track metadata endpoint. First request hits Postgres and populates Redis with a 1-hour TTL. Subsequent requests are served from Redis in ~2ms vs ~15ms from DB. At 325k requests in our load test, we had 3 cache misses total — one per API instance on cold start. The stream endpoint itself isn't cached because each pre-signed URL is per-user and short-lived, but the metadata lookup before generating the URL is cached.

**"How did you implement rate limiting and what algorithm did you use?"**
> Token bucket. Each unauthenticated IP gets 100 tokens per hour, refilling continuously at 100/3600 per second. The key insight is that the read-modify-write has to be atomic — I use a Redis Lua script so all three API instances share a single consistent bucket per IP without race conditions. If Redis goes down, I fail open rather than blocking all traffic.

**"Why least_conn instead of round-robin?"**
> Round-robin assumes all requests take the same time. With audio streaming, some requests are fast (metadata lookup, 2ms) and some are slower (stream redirect with DB query, 15ms). Least_conn routes new requests to whichever instance has the fewest in-flight connections, so a slow request on one instance doesn't cause that instance to fall behind.
