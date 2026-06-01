# System Design Lab
> Learn system design by building a real product from scratch.
> Every concept — rate limiting, consistent hashing, load balancing, database design — is taught
> through hands-on implementation, not slides. The capstone is a working Spotify-like audio
> streaming service called **Beatstream**.

---

## Table of Contents

**Foundations**
- [How to use this guide](#how-to-use-this-guide)
- [System design vocabulary](#system-design-vocabulary)

**Core Concepts** _(understand before you build)_
- [Network basics](#network-basics)
- [API design](#api-design)
- [Databases](#databases)
- [Caching](#caching)
- [Message queues](#message-queues)
- [Load balancing](#load-balancing)
- [Rate limiting](#rate-limiting)
- [Consistent hashing](#consistent-hashing)
- [CAP theorem](#cap-theorem)
- [Availability & reliability](#availability--reliability)
- [Scalability patterns](#scalability-patterns)
- [SLOs, SLAs, and error budgets](#slos-slas-and-error-budgets)

**Infrastructure** _(how to run it)_
- [Docker & containers](#docker--containers)
- [Kubernetes](#kubernetes)
- [Infrastructure as code](#infrastructure-as-code)
- [Observability](#observability)

**Capstone: Beatstream** _(build it end-to-end)_
- [Quickstart](beatstream/README.md)
- [Phase 0 — local monolith](docs/phase-0.md)
- [Phase 1 — load balancing & caching](docs/phase-1.md)
- [Phase 2 — async & queues](docs/phase-2.md)
- [Phase 3 — kubernetes](docs/phase-3.md)
- [Phase 4 — frontend (Next.js)](docs/phase-4.md)
- [Phase 5 — JWT authentication](docs/phase-5.md)
- [Phase 6 — security foundations](docs/phase-6.md)
- [Phase 7 — RBAC + GDPR](docs/phase-7.md)
- [Phase 8 — AWS cloud deployment](docs/phase-8.md)

**Interview Prep**
- [Classic questions](#classic-interview-questions)
- [Learning journal](#learning-journal)

---

## How to use this guide

This repo has two parts that feed each other.

**Part 1 — Core Concepts.** Read one section. Understand the trade-offs, not just the definition.
Each section ends with the exact interview question it prepares you for.

**Part 2 — Beatstream.** A minimal Spotify clone you build phase by phase.
Every concept from Part 1 shows up as a real design decision in Beatstream.

The rule: **don't move to the next phase until you can answer the interview question at the bottom
of the current one.** Not from memory. From having debugged it at 2am.

```
Read concept → implement in Beatstream → get it wrong → fix it → now you own it
```

---

## System design vocabulary

Before anything else, internalize these terms. They appear in every interview.

| Term | One-line definition |
|------|-------------------|
| Latency | Time for one operation to complete |
| Throughput | Operations completed per unit time |
| Availability | % of time the system is operational |
| Consistency | All nodes see the same data at the same time |
| Partition tolerance | System works despite network splits |
| Scalability | System handles growth without redesign |
| Reliability | System behaves correctly over time |
| Fault tolerance | System continues working when components fail |
| Idempotency | Repeating an operation produces the same result |
| Durability | Committed data survives failures |
| Eventual consistency | Nodes converge to same state given enough time |
| Strong consistency | Every read sees the most recent write |
| Backpressure | Downstream signals upstream to slow down |

---

# Network basics

## DNS

When you type `beatstream.io` into a browser, DNS resolves it to an IP address.

```
Browser → Recursive resolver → Root nameserver
       → TLD nameserver (.io) → Authoritative nameserver
       → IP address: 54.32.11.100
```

**TTL (Time To Live):** How long resolvers cache the answer.
- Low TTL (30s): fast failover, more DNS queries
- High TTL (86400s): fast resolution, slow failover

**Beatstream relevance:** During a blue/green deployment, you flip the DNS record to point to the
new version. A low TTL means clients switch quickly; a high TTL means some clients talk to the old
version for a long time. You'll feel this in Phase 4.

## CDN

A Content Delivery Network is a globally distributed network of cache servers (PoPs — Points of
Presence). Clients are routed to the nearest PoP.

```
User in Taiwan → CloudFront PoP in Singapore → (cache hit) → audio file
                                              → (cache miss) → S3 origin → cache → user
```

**Why audio streaming needs a CDN:**
- A 4-minute song at 320kbps is ~10MB. Multiply by 10,000 concurrent streams.
- Serving that from a single origin in us-east-1 to users in Asia: ~200ms latency.
- Serving from a Singapore PoP: ~5ms. Night and day.

**Cache-Control header drives CDN behavior:**
```
Cache-Control: public, max-age=86400    ← CDN caches for 24h
Cache-Control: private, no-store        ← CDN never caches (for auth tokens)
```

**Interview question this covers:**
> *"How would you reduce latency for users downloading files from a global service?"*

---

# API design

## REST

REST is an architectural style, not a protocol. The constraints that matter:

- **Stateless:** Every request contains all information needed to process it.
  The server stores nothing about client state between requests.
- **Uniform interface:** Resources are identified by URLs. Operations are HTTP verbs.
- **Client-server separation:** UI and data storage are decoupled.

**Resource modeling for Beatstream:**
```
GET    /v1/tracks/{id}                 → get a track
GET    /v1/tracks/{id}/stream          → stream audio bytes (range requests)
POST   /v1/playlists                   → create playlist
PUT    /v1/playlists/{id}              → replace playlist
PATCH  /v1/playlists/{id}             → partial update
DELETE /v1/playlists/{id}             → delete
GET    /v1/search?q=radiohead&type=artist,track  → search
GET    /v1/me/recently-played          → user's history
```

**Never do this:**
```
GET  /getTrack?id=123       ← verbs in URLs
POST /deletePlaylist/456    ← wrong method for deletion
GET  /v1/tracks/123/doPlay  ← actions as nouns
```

## Pagination

**Offset pagination** — simple, breaks under concurrent writes:
```
GET /v1/tracks?offset=100&limit=20
Problem: if a track is inserted at position 50 while you page,
         you skip one track at offset 100 and see a duplicate at 120.
```

**Cursor pagination** — stable, production-standard:
```
GET /v1/playlists/{id}/tracks?limit=50&after=dXNlcjoxMjM0
Response:
{
  "items": [...],
  "next_cursor": "dXNlcjoxMjM2",
  "has_more": true
}
The cursor encodes the last seen ID (base64). Insertion/deletion doesn't break it.
```

## Idempotency

An operation is idempotent if repeating it has the same effect as doing it once.

```
GET    → always idempotent (read-only)
PUT    → idempotent (replace resource to this exact state)
DELETE → idempotent (deleted twice = deleted)
POST   → NOT idempotent by default

Making POST idempotent:
  Client generates UUID, sends as Idempotency-Key header.
  Server stores (key → response) in Redis for 24h.
  Duplicate request hits cache, returns same response.
  Network retry is safe: no double-charge, no double-like.
```

**Beatstream uses this for:** the `/v1/swipe` endpoint (prevent double-liking).

## Versioning

```
URL versioning (breaking changes):   /v1/tracks  →  /v2/tracks
Header versioning (additive):        API-Version: 2025-01-01
Deprecation lifecycle:
  1. Release v2
  2. Add Sunset header to v1: Sunset: Sat, 01 Jun 2026 00:00:00 GMT
  3. v1 returns 410 Gone after sunset date
```

**Interview questions this covers:**
> *"Design the API for a music streaming service."*
> *"How do you handle API versioning without breaking existing clients?"*

---

# Databases

## Relational vs non-relational

Choosing a database is a trade-off, not a preference.

| | PostgreSQL (relational) | MongoDB (document) | Redis (key-value) |
|--|------------------------|-------------------|------------------|
| Data model | Tables, rows, joins | JSON documents | Keys → values |
| Schema | Strict, enforced | Flexible | None |
| ACID | Full | Per-document | Limited |
| Joins | Native, efficient | Application-side | None |
| Scale | Vertical + read replicas | Horizontal sharding | Horizontal |
| Best for | Financial records, user data | Catalogs, content | Sessions, cache, queues |

**Beatstream schema design (PostgreSQL):**
```sql
CREATE TABLE tracks (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title         TEXT NOT NULL,
  artist_id     UUID NOT NULL REFERENCES artists(id),
  duration_ms   INT NOT NULL,
  audio_url     TEXT NOT NULL,       -- S3 key
  release_date  DATE,
  play_count    BIGINT DEFAULT 0,
  created_at    TIMESTAMPTZ DEFAULT now(),
  -- Full-text search vector (auto-maintained)
  search_vector TSVECTOR GENERATED ALWAYS AS (
    setweight(to_tsvector('english', title), 'A')
  ) STORED
);
CREATE INDEX tracks_search_idx  ON tracks USING GIN(search_vector);
CREATE INDEX tracks_artist_idx  ON tracks(artist_id);
CREATE INDEX tracks_release_idx ON tracks(release_date DESC);

-- Play events: partitioned by month (efficient range scans + pruning)
CREATE TABLE play_events (
  user_id    UUID NOT NULL,
  track_id   UUID NOT NULL,
  played_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  ms_played  INT,
  source     TEXT                  -- 'playlist', 'search', 'radio'
) PARTITION BY RANGE (played_at);
CREATE TABLE play_events_2025_06 PARTITION OF play_events
  FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');
```

## Indexing

An index is a separate data structure that maps column values to row locations.
Without it, every query is a full table scan O(n). With it, lookups are O(log n).

```
Query: SELECT * FROM tracks WHERE artist_id = 'abc';
Without index:  read all 50M rows, check each → 12 seconds
With index:     B-tree lookup → 0.3ms

The catch: every index slows down INSERT/UPDATE/DELETE.
           An over-indexed table can write slower than it reads.
           Only index columns you actually filter or sort on.
```

**Index types and when to use them:**

| Type | Use case | Example |
|------|----------|---------|
| B-Tree | Equality, range, ORDER BY | `tracks(release_date)` |
| GIN | Full-text, arrays, JSONB | `tracks(search_vector)` |
| GiST | Geo, range overlap | `users(location)` |
| BRIN | Large, append-only, time-ordered | `play_events(played_at)` |
| Partial | Sparse condition | `WHERE deleted_at IS NULL` |
| Composite | Multi-column filter | `(artist_id, release_date)` |

## Read replicas

```
Single primary:
  All reads + writes hit one server.
  At 10,000 req/s, the DB becomes the bottleneck.

With read replicas:
  Writes → Primary (single source of truth)
  Reads  → Replica 1, Replica 2  (async replication, ~10-100ms lag)

Application routing:
  db.query("SELECT...", read_preference="replica")   ← 80% of queries
  db.query("INSERT...", read_preference="primary")   ← all writes
  db.query("SELECT...", read_preference="primary")   ← after a write (lag-sensitive)

Beatstream: 95% of requests are reads (browsing catalog, streaming).
            Two read replicas handle the load; primary handles writes only.
```

## Connection pooling

```
Problem:
  Each DB connection uses ~5MB RAM on the PostgreSQL server.
  max_connections = 100 (default).
  With 10 API servers × 20 connections each = 200 connections → crash.

Solution — PgBouncer (transaction-mode pooling):
  API servers connect to PgBouncer (lightweight proxy).
  PgBouncer maintains a small pool of real DB connections.
  1000 app connections → 20 actual DB connections.

Pool size formula: (cpu_cores × 2) + effective_spindle_count
  A db.r6g.large (2 vCPU) → pool_size = 5 to 10
```

**Interview questions this covers:**
> *"How would you design the database for a music streaming service?"*
> *"How do you scale a relational database?"*
> *"What's the difference between a B-tree and a GIN index?"*

---

# Caching

## Why cache

```
Latency comparison:
  L1 cache:          0.5 ns
  L2 cache:          7 ns
  RAM:               100 ns
  SSD:               150 µs
  Network (same DC): 500 µs
  PostgreSQL query:  1-20 ms
  Redis GET:         0.2-0.5 ms   ← 20-100x faster than DB
  Cross-region:      30-150 ms
```

## Cache strategies

**Cache-aside (lazy loading)** — most common:
```python
def get_track(track_id):
    cached = redis.get(f"track:{track_id}")
    if cached:
        return json.loads(cached)              # cache hit
    track = db.query("SELECT * FROM tracks WHERE id = ?", track_id)
    redis.setex(f"track:{track_id}", 3600, json.dumps(track))   # populate
    return track                               # cache miss → DB → populate
```

**Write-through** — cache updated on every write:
```python
def update_track(track_id, data):
    db.update(track_id, data)
    redis.setex(f"track:{track_id}", 3600, json.dumps(data))  # sync update
    # Pro: cache always fresh. Con: write penalty on every update.
```

**Write-behind (write-back)** — write to cache, async to DB:
```python
# Used for play_count: increment Redis counter, flush to DB every 60s.
# Pro: absorbs write spikes. Con: data loss if Redis crashes before flush.
redis.incr(f"play_count:{track_id}")
# Background job: every 60s, flush Redis counters to PostgreSQL
```

## Cache eviction

When the cache is full, something must be evicted:

| Policy | Behavior | Use case |
|--------|----------|----------|
| LRU | Evict least recently used | General purpose |
| LFU | Evict least frequently used | Zipf-distributed access (tracks) |
| TTL | Evict after time expires | Session data, auth tokens |
| Random | Evict random key | Simple, surprisingly effective |

**Beatstream:** Uses LFU for track metadata. The top 0.1% of tracks get 80% of streams.
LFU keeps Radiohead's back catalog hot; TTL cleans up one-time search results.

## Cache stampede (thundering herd)

```
Problem:
  Popular track's cache TTL expires.
  10,000 concurrent requests all miss the cache simultaneously.
  All 10,000 hit PostgreSQL at once. DB falls over.

Solution 1 — Mutex lock:
  First request acquires Redis lock → fetches from DB → populates cache.
  Other 9,999 requests wait on lock → then read from (now populated) cache.

Solution 2 — Probabilistic early expiration:
  While item is still valid, randomly decide to refresh early:
  if (now - last_computed) > (ttl - β × log(random()))
      refresh_in_background()
  Distributes refreshes over time. No thundering herd.

Solution 3 — Stale-while-revalidate:
  Serve stale data immediately (no wait).
  Async: refresh cache in background.
  Used by Beatstream for track metadata. Slightly stale is fine.
```

**Interview questions this covers:**
> *"How do you cache data effectively and avoid stale reads?"*
> *"What is a cache stampede and how do you prevent it?"*

---

# Message queues

## Why queues

```
Without a queue:
  User uploads a song → API synchronously:
    1. Validate audio format      (10ms)
    2. Transcode to MP3/AAC/OGG   (8 seconds!)
    3. Generate waveform          (2 seconds)
    4. Extract metadata           (200ms)
    5. Upload to S3               (500ms)
    6. Update database            (20ms)
    Total: ~11 seconds blocking. User times out. API fails.

With a queue:
  User uploads a song → API:
    1. Validate audio format      (10ms)
    2. Push job to queue          (1ms)
    3. Return 202 Accepted immediately
  Background worker processes the rest asynchronously.
  User gets a webhook/notification when ready.
```

## Kafka fundamentals

```
Architecture:
  Producers → Topics (partitioned, replicated) → Consumer groups

Topic: play-events
  Partition 0: [event1, event4, event7, ...]  → Consumer A
  Partition 1: [event2, event5, event8, ...]  → Consumer B
  Partition 2: [event3, event6, event9, ...]  → Consumer C

Guarantees:
  Within a partition: strict ordering
  Across partitions: no ordering guarantee
  Retention: events stored for N days (not deleted on consume)
  Consumer groups: each group reads independently (replay, fan-out)
```

**Beatstream Kafka topics:**
```
play-events        → analytics aggregation, recommendation updates
track-uploads      → transcoding workers
user-events        → "new follower", "playlist added" notifications
```

**At-least-once vs exactly-once:**
```
At-least-once (default):
  Consumer commits offset after processing.
  On crash before commit: message redelivered → processed twice.
  Solution: make your processing idempotent.

Exactly-once:
  Kafka transactions + idempotent producer.
  Higher latency, more complex.
  Use for financial transactions. Not needed for play counts.
```

**Interview questions this covers:**
> *"How do you handle background job processing at scale?"*
> *"Explain Kafka partitioning and why it matters."*
> *"What's the difference between a queue and a pub/sub system?"*

---

# Load balancing

## Algorithms

```
Round Robin:
  Request 1 → Server A
  Request 2 → Server B
  Request 3 → Server C
  Request 4 → Server A ...
  Best for: homogeneous servers, short equal requests.

Weighted Round Robin:
  Server A (weight 3): gets 3x more requests than Server B (weight 1).
  Best for: heterogeneous hardware.

Least Connections:
  New request → server with fewest active connections.
  Best for: long-lived connections, variable request duration.
  Beatstream uses this for WebSocket connections.

Least Response Time:
  New request → server with lowest avg response time.
  Best for: latency-sensitive APIs.

IP Hash:
  hash(client_ip) % num_servers → always same server.
  Provides session affinity. Bad distribution if traffic is from one IP range.
  Avoid unless you need sticky sessions.
```

## Health checks

```
Passive (circuit breaker):
  Load balancer observes real traffic responses.
  If server returns 5xx or times out N times → mark unhealthy → stop routing.
  Lag: N failures must happen first. Doesn't catch "slow but not failing."

Active:
  Load balancer polls GET /health every 5s.
  If no 200 response → mark unhealthy immediately.
  Combine both: active for fast detection, passive for ongoing monitoring.

/health endpoint contract:
  200 OK  → healthy, send me traffic
  503     → unhealthy (DB unreachable, cache full)
  Body should check: DB connection, Redis connection, disk space
```

## Nginx configuration

```nginx
upstream beatstream_api {
    least_conn;
    server api-1:8080 weight=1 max_fails=3 fail_timeout=30s;
    server api-2:8080 weight=1 max_fails=3 fail_timeout=30s;
    server api-3:8080 weight=1 max_fails=3 fail_timeout=30s;
    keepalive 32;   # persistent connections to upstream
}

server {
    listen 80;
    location /api/ {
        proxy_pass         http://beatstream_api;
        proxy_read_timeout 30s;
        proxy_set_header   X-Request-ID $request_id;
        proxy_set_header   X-Real-IP    $remote_addr;
    }
    location /health {
        access_log off;
        return 200 "ok\n";
    }
}
```

**Interview questions this covers:**
> *"How does a load balancer work?"*
> *"What algorithm would you use for load balancing WebSocket connections?"*
> *"How do you do zero-downtime deployments?"*

---

# Rate limiting

## Why rate limiting

Without rate limits:
- One client can exhaust your server resources (intentional or not)
- A bug in a mobile app retrying aggressively can DDoS your own service
- Prevents one free-tier user from consuming premium resources

## Algorithms

**Token bucket** — allows bursting:
```
Each user has a bucket with capacity N tokens.
Tokens refill at R tokens/second.
Each request consumes 1 token.
If bucket empty → 429 Too Many Requests.

Beatstream /stream: capacity=20, refill=2/sec
  → burst of 20 requests, then steady 2/sec
  → makes seeking feel instant, prevents runaway clients
```

**Sliding window log** — exact, memory-intensive:
```
Store timestamp of each request for the past window.
Count entries in window → if >= limit → reject.
Exact, but stores every event. O(n) memory per user.
```

**Sliding window counter** — approximate, memory-efficient:
```
Two counters: current window, previous window.
current_count + (prev_count × overlap_ratio) < limit → allow
Overlap ratio = how much of the previous window overlaps current request time
At request time 0.7s into a 1s window: ratio = 0.3
Memory: O(1) per user. Good enough for API rate limiting.
```

## Redis Lua implementation

```lua
-- Atomic token bucket in Redis
-- Keys[1]: rate_limit:{user_id}:{endpoint}
-- Argv: max_tokens, refill_rate, now, cost
local key         = KEYS[1]
local max_tokens  = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now         = tonumber(ARGV[3])
local cost        = tonumber(ARGV[4])

local data        = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens      = tonumber(data[1]) or max_tokens
local last_refill = tonumber(data[2]) or now

-- Refill
local elapsed = now - last_refill
tokens = math.min(max_tokens, tokens + elapsed * refill_rate)

if tokens < cost then
  return {0, math.ceil((cost - tokens) / refill_rate)}  -- {denied, retry_after}
end

tokens = tokens - cost
redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, math.ceil(max_tokens / refill_rate) * 2)
return {1, 0}   -- {allowed}
```

**Why Lua?** Lua scripts execute atomically in Redis. Without it, a race condition between
`GET tokens` and `SET tokens` allows two requests to both pass when only one should.

**Interview questions this covers:**
> *"Design a rate limiter."*
> *"How do you implement rate limiting in a distributed system?"*
> *"Why is the token bucket better than a fixed window counter?"*

---

# Consistent hashing

## The problem

```
5 Redis nodes. Naive sharding: node = hash(key) % 5
Add a 6th node: node = hash(key) % 6
→ Almost every key maps to a different node.
→ Cache miss rate spikes to ~83% while data migrates.
→ Database gets hammered. This has taken down real services.
```

## The solution

```
Hash ring: imagine values 0 to 2^32 arranged in a circle.
Each node is placed at multiple positions (virtual nodes):
  node-1-v1, node-1-v2, ..., node-1-v150
  node-2-v1, node-2-v2, ..., node-2-v150

To find which node owns a key:
  1. hash(key) → position on ring
  2. Walk clockwise → first node position encountered
  3. That node's physical server handles this key

Adding node-6:
  Only keys between node-6 and its predecessor remap.
  ~1/N keys migrate (instead of all of them).
  Cache miss spike: ~17% instead of ~83%.
```

```go
type Ring struct {
    nodes      map[uint32]string  // ring position → node name
    sorted     []uint32           // sorted positions for binary search
    vnodes     int                // virtual nodes per physical node
}

func (r *Ring) Get(key string) string {
    h    := crc32.ChecksumIEEE([]byte(key))
    idx  := sort.Search(len(r.sorted), func(i int) bool {
        return r.sorted[i] >= h
    })
    if idx == len(r.sorted) { idx = 0 }   // wrap around
    return r.nodes[r.sorted[idx]]
}
```

**Beatstream uses consistent hashing for:** distributing track metadata across a Redis cluster.
Adding a cache node during a traffic spike remaps only 1/N of keys, not all of them.

**Interview questions this covers:**
> *"How does consistent hashing work?"*
> *"How would you add a new node to a distributed cache without a thundering herd?"*

---

# CAP theorem

A distributed system can guarantee at most two of:
- **Consistency** — every read returns the most recent write (or an error)
- **Availability** — every request gets a response (not an error)
- **Partition tolerance** — system works despite network splits between nodes

**The catch:** partition tolerance is mandatory in real distributed systems.
Networks fail. You always have to choose between C and A when a partition occurs.

```
CP systems (consistent, partition-tolerant):
  HBase, Zookeeper, etcd
  On partition: reject writes to preserve consistency.
  Use when: financial transactions, inventory counts, leader election.

AP systems (available, partition-tolerant):
  Cassandra, DynamoDB (default), CouchDB
  On partition: serve possibly stale data to stay available.
  Use when: social feeds, product catalogs, shopping carts.

Beatstream:
  Track catalog → AP (stale metadata for 30s is fine)
  Play count    → AP (approximate counts are acceptable)
  Payment/subscription → CP (must be consistent)
```

**Interview questions this covers:**
> *"Explain CAP theorem."*
> *"Is your system CP or AP and why did you choose that?"*

---

# Availability & reliability

## Nines

| Availability | Downtime per year | Downtime per month |
|-------------|-------------------|-------------------|
| 99% (2 nines) | 3.65 days | 7.3 hours |
| 99.9% (3 nines) | 8.76 hours | 43.8 minutes |
| 99.99% (4 nines) | 52.6 minutes | 4.38 minutes |
| 99.999% (5 nines) | 5.26 minutes | 26.3 seconds |

Going from 99.9% to 99.99% is an order of magnitude harder and more expensive.
Most services target 99.9% or 99.95%. Five nines is for air traffic control.

## Fault tolerance patterns

**Circuit breaker:**
```
States: CLOSED → OPEN → HALF-OPEN → CLOSED

CLOSED: requests flow normally, failures counted.
OPEN:   failure rate exceeded threshold. All requests fail fast (no waiting).
        After timeout, move to HALF-OPEN.
HALF-OPEN: one probe request allowed through.
           If succeeds → CLOSED. If fails → OPEN.

Beatstream: wraps the recommendation service.
If it's down, serve cached recommendations. Don't let it cascade.
```

**Retry with exponential backoff:**
```
Attempt 1: fail → wait 100ms
Attempt 2: fail → wait 200ms
Attempt 3: fail → wait 400ms + jitter (±50ms random)
Attempt 4: fail → wait 800ms + jitter
Attempt 5: give up, return error

Jitter prevents synchronized retries (thundering herd on retries).
Never retry non-idempotent operations without an idempotency key.
```

**Bulkhead:**
```
Isolate failures to a bounded pool.
Beatstream has separate thread pools for:
  - Audio streaming (large, latency-critical)
  - Catalog browsing (medium)
  - Recommendation engine (can be slow, isolated)

If recommendations are slow, they consume their pool.
Audio streaming is unaffected.
```

---

# Scalability patterns

## Horizontal vs vertical scaling

```
Vertical (scale up): bigger machine.
  Pros: simple, no code changes, works for stateful services.
  Cons: expensive, has a ceiling, single point of failure.

Horizontal (scale out): more machines.
  Pros: theoretically unlimited, cheap commodity hardware, fault tolerant.
  Cons: requires stateless design, more operational complexity.

Rule: scale vertically until the cost or ceiling forces you horizontal.
```

## Stateless design

```
Stateful (bad for horizontal scaling):
  User's session stored in memory on Server A.
  Load balancer routes their next request to Server B.
  Server B doesn't know about their session. Error.

Stateless (good):
  Session stored in Redis (external store).
  Any server can handle any request.
  Add or remove servers freely.

Beatstream API: all state in PostgreSQL + Redis.
                API pods are stateless. HPA can scale them freely.
```

## Database scaling

```
Read replicas → Handle read-heavy workload (Beatstream: 95% reads)
Partitioning  → Split large tables by range, hash, or list
Sharding      → Split across multiple DB servers by shard key
CQRS          → Separate read model from write model

Beatstream sharding strategy for play_events:
  Shard key: user_id
  Shard 0: user_ids 00000000-3FFFFFFF
  Shard 1: user_ids 40000000-7FFFFFFF
  Shard 2: user_ids 80000000-BFFFFFFF
  Shard 3: user_ids C0000000-FFFFFFFF
```

---

# SLOs, SLAs, and error budgets

## Definitions

**SLI (Service Level Indicator):** a metric. p99 latency = 180ms.
**SLO (Service Level Objective):** a target. p99 latency < 200ms, measured over 30 days.
**SLA (Service Level Agreement):** a contract with a customer.
If we miss the SLA, they get a refund/credit. The SLO is always stricter than the SLA.

**Error budget:** (1 - SLO target) × window duration.
At 99.9% availability over 30 days: 43.8 minutes of allowed downtime.

## Error budget policy

```
Error budget remaining:
  > 50%  → Deploy freely. Experiment. Ship fast.
  25-50% → Deploy with caution. Extra review for risky changes.
  10-25% → No non-critical deploys. Focus on reliability.
  < 10%  → Feature freeze. All hands on reliability.
  0%     → Incident review required before any deploy.
```

## Beatstream SLO definition

```yaml
service: beatstream-api
window: 30d
slos:
  - name: availability
    sli: successful_requests / total_requests
    target: 99.9%
    alert_at: 10% budget remaining
  - name: stream_latency_p99
    sli: histogram_quantile(0.99, stream_ttfb_seconds)
    threshold: 500ms
    target: 99%
  - name: search_latency_p99
    sli: histogram_quantile(0.99, search_duration_seconds)
    threshold: 200ms
    target: 99.5%
```

**Interview questions this covers:**
> *"What is an SLO and how is it different from an SLA?"*
> *"How do you use error budgets to make deployment decisions?"*

---

# Docker & containers

## Why containers

```
"Works on my machine" → container packages the machine.
A container bundles:
  - your application binary
  - its dependencies
  - its runtime (libc, Python version, etc.)
  - its config

Every environment (dev laptop, CI, staging, production) runs
the same image. No "it worked in dev" failures.
```

## Key concepts

```
Image:     immutable snapshot. Built from a Dockerfile. Tagged and versioned.
Container: running instance of an image. Ephemeral. State goes in volumes.
Volume:    persistent storage mounted into a container.
Network:   virtual network connecting containers.

Dockerfile for Beatstream API:
  FROM golang:1.22-alpine AS builder
  WORKDIR /app
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN go build -o beatstream-api ./cmd/api

  FROM alpine:3.19                    # small final image
  RUN apk add --no-cache ca-certificates
  COPY --from=builder /app/beatstream-api /beatstream-api
  EXPOSE 8080
  CMD ["/beatstream-api"]
```

See `beatstream/docker-compose.yml` for the full local development stack.

---

# Kubernetes

## Core objects

```
Pod:        smallest deployable unit. One or more containers.
Deployment: manages N identical pods. Handles rollout, rollback.
Service:    stable network endpoint for a set of pods.
Ingress:    routes external HTTP(S) traffic to services.
ConfigMap:  non-secret config (env vars, files).
Secret:     sensitive config (passwords, API keys).
HPA:        auto-scales pods based on metrics.
PVC:        persistent disk storage for stateful pods.
```

## HPA — Horizontal Pod Autoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: beatstream-api-hpa
spec:
  scaleTargetRef:
    kind: Deployment
    name: beatstream-api
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 30     # don't wait to scale up
    scaleDown:
      stabilizationWindowSeconds: 300    # wait 5 min before shrinking
```

## Probes

```yaml
livenessProbe:         # is this container alive? If no → restart it.
  httpGet:
    path: /healthz
    port: 8080
  failureThreshold: 3
  periodSeconds: 10

readinessProbe:        # is this container ready for traffic? If no → remove from Service.
  httpGet:
    path: /ready       # returns 200 only when DB + Redis connections are established
    port: 8080
  failureThreshold: 3
  periodSeconds: 5

# Critical distinction:
#   liveness  → "restart me"     (process stuck in deadlock)
#   readiness → "stop sending traffic" (DB connection pool exhausted)
# A pod can be alive but not ready. That's normal during startup.
```

---

# Infrastructure as code

## Why Terraform

Manual cloud setup is:
- Not reproducible (you forget a checkbox in the console)
- Not reviewable (no git history, no PR review)
- Not recoverable (if you accidentally delete it, you rebuild from memory)

Terraform encodes your entire infrastructure in version-controlled `.tf` files.

```hcl
# Create an ECS service with 3 tasks behind an ALB
resource "aws_ecs_service" "beatstream_api" {
  name            = "beatstream-api"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = 3

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true    # auto-rollback if new deploy fails health checks
  }
}
```

**Workflow:**
```
terraform plan    → show what will change (diff)
terraform apply   → apply changes
terraform destroy → tear down (use with caution)
State stored in S3 + locked with DynamoDB to prevent concurrent applies.
```

See `beatstream/infra/terraform/` for the Phase 4 implementation.

---

# Observability

## The three pillars

**Metrics** — What is happening? (numbers over time)
```
Rate:       requests per second
Errors:     error rate (4xx, 5xx)
Duration:   p50, p95, p99 latency
Saturation: CPU %, memory %, DB connection pool usage
```

**Logs** — What happened? (discrete events)
```json
{
  "time": "2025-06-01T14:23:01Z",
  "level": "error",
  "request_id": "01HXK9...",
  "user_id": "usr_abc",
  "endpoint": "GET /v1/tracks/123/stream",
  "status": 503,
  "latency_ms": 5001,
  "error": "upstream connection timeout",
  "trace_id": "abc123"
}
```

**Traces** — Why did it happen? (request flow across services)
```
Trace: GET /v1/tracks/123/stream   total: 340ms
  ├─ auth.ValidateJWT              3ms
  ├─ redis.GetTrack                0.4ms   ← cache hit
  ├─ s3.GeneratePresignedURL       12ms
  └─ response                      2ms

If cache miss:
  ├─ auth.ValidateJWT              3ms
  ├─ redis.GetTrack                0.4ms   ← cache miss
  ├─ postgres.QueryTrack           24ms    ← goes to DB
  ├─ redis.SetTrack                0.5ms
  ├─ s3.GeneratePresignedURL       12ms
  └─ response                      2ms
```

## Prometheus + Grafana setup

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'beatstream-api'
    static_configs:
      - targets: ['api-1:9090', 'api-2:9090', 'api-3:9090']
    scrape_interval: 15s

# Grafana dashboard queries (PromQL)
Request rate:  rate(http_requests_total[5m])
Error rate:    rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m])
P99 latency:   histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))
```

---

# Capstone: Beatstream

> A minimal Spotify clone you build phase by phase.
> At the end: real audio streaming, search, playlists, and a load-tested, monitored service.

## Architecture overview

```
Client (browser/mobile)
  │
  ▼
CDN (CloudFront)                ← static assets + audio file cache
  │
  ▼
API Gateway / Nginx             ← TLS termination, rate limiting, routing
  │
  ├──▶ Beatstream API (Go)      ← stateless, horizontally scalable
  │         │
  │         ├──▶ PostgreSQL     ← tracks, users, playlists, play events
  │         ├──▶ Redis          ← cache, sessions, rate limiting
  │         └──▶ S3             ← audio files (pre-signed URLs)
  │
  └──▶ Kafka                    ← play events → analytics, recommendations
            │
            └──▶ Workers        ← transcoding, recommendations, notifications
```

## Tech stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Language | Go | Fast, small containers, great for HTTP servers |
| HTTP router | Gin | Minimal, well-documented, fast |
| Database | PostgreSQL 16 | ACID, full-text search, great indexing |
| Cache | Redis 7 | Rate limiting, session, metadata cache |
| Object storage | S3 (or MinIO local) | Audio files, album art |
| Message queue | Kafka (or Redpanda local) | Play event streaming |
| Load balancer | Nginx | Battle-tested, easy local setup |
| Containers | Docker + Docker Compose | Local dev |
| Orchestration | Kubernetes (minikube → EKS) | Dynamic scaling |
| IaC | Terraform | Reproducible cloud infra |
| Metrics | Prometheus + Grafana | Standard stack |
| Tracing | Jaeger (OpenTelemetry) | Request-level debugging |
| Load testing | k6 | Scriptable, realistic load patterns |

---

## Phase 0 — Local monolith

**Goal:** Get something running. One binary, one database, no tricks.
**Estimated time:** 1 week
**Concept from Part 1:** API design, database schema

### What you build

```
Monolith API (Go)
  ├── GET  /v1/tracks/:id
  ├── GET  /v1/tracks/:id/stream     (redirect to S3 pre-signed URL)
  ├── GET  /v1/search?q=
  ├── POST /v1/playlists
  ├── GET  /v1/playlists/:id/tracks
  └── POST /v1/playlists/:id/tracks

PostgreSQL (local Docker)
MinIO (local S3 replacement)
```

### Repo structure

```
beatstream/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── handler/
│   │   ├── tracks.go
│   │   ├── playlists.go
│   │   └── search.go
│   ├── db/
│   │   └── postgres.go
│   ├── storage/
│   │   └── s3.go
│   └── middleware/
│       └── requestid.go
├── db/
│   └── migrations/
│       ├── 001_create_tracks.sql
│       ├── 002_create_playlists.sql
│       └── 003_create_play_events.sql
├── docker-compose.yml
└── Makefile
```

### Milestones

```
[ ] docker compose up brings postgres + minio + api
[ ] POST /v1/tracks with audio file → stored in MinIO, metadata in Postgres
[ ] GET  /v1/tracks/:id/stream → returns pre-signed URL (valid 1 hour)
[ ] GET  /v1/search?q=radiohead → full-text search, results ranked by play_count
[ ] GET  /v1/playlists/:id/tracks → cursor-paginated response
[ ] All endpoints return structured JSON errors (not stack traces)
[ ] /healthz and /ready endpoints
```

### Interview question to answer before Phase 1

> *"Walk me through the database schema you chose and why."*
> *"How does pre-signed URL audio streaming work? Why not proxy the audio through your server?"*

---

## Phase 1 — Load balancing & caching

**Goal:** Feel what changes when you put load on it. Fix the bottlenecks.
**Estimated time:** 1 week
**Concepts from Part 1:** Caching, load balancing, rate limiting

### What you add

```
Nginx upstream (3 API instances)    ← load balancing
Redis cache layer                   ← track metadata, search results
Token bucket rate limiter           ← on /v1/tracks/:id/stream
k6 load test script                 ← to see the difference
```

### k6 load test

```javascript
// k6/load.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('error_rate');

export const options = {
  stages: [
    { duration: '30s', target: 50  },   // ramp up
    { duration: '2m',  target: 200 },   // sustained load
    { duration: '30s', target: 500 },   // stress
    { duration: '30s', target: 0   },   // ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(99)<200'],
    'error_rate':        ['rate<0.01'],
  },
};

export default function () {
  const trackId = __ENV.TRACK_ID;
  const res = http.get(`http://localhost/v1/tracks/${trackId}`);
  check(res, { 'status 200': (r) => r.status === 200 });
  errorRate.add(res.status !== 200);
  sleep(0.1);
}
```

### Milestones

```
[ ] nginx.conf with 3 upstreams, least_conn, health checks
[ ] Redis cache for track metadata (cache-aside, 1h TTL)
[ ] Redis cache for search results (1min TTL)
[ ] Rate limiter middleware: 100 streams/hour for unauthenticated, unlimited for auth
[ ] k6 run: see p99 latency drop from ~50ms to ~2ms after cache warms up
[ ] Grafana dashboard: request rate, error rate, p99 latency, cache hit rate
[ ] Observe: what happens when you kill one API instance mid-test?
[ ] Observe: what happens when Redis goes down?
```

### Interview question to answer before Phase 2

> *"Your /stream endpoint is getting hammered. Walk me through your caching strategy."*
> *"How did you implement rate limiting and what algorithm did you use?"*

---

## Phase 2 — Async & queues

**Goal:** Decouple upload processing. Handle play events at scale.
**Estimated time:** 1 week
**Concepts from Part 1:** Message queues, async processing

### What you add

```
Kafka (or Redpanda local)
  Topic: play-events   → consumed by analytics worker
  Topic: track-uploads → consumed by transcoding worker

Play event flow:
  User streams track → API publishes play-event → Kafka
                                              → Analytics worker updates play_count
                                              → (future) Recommendation worker

Upload flow (async):
  User uploads MP3 → API validates (fast) → publishes to track-uploads → 202 Accepted
                                         → Worker transcodes → updates track status
                                         → User polls /v1/tracks/:id/status
```

### Milestones

```
[ ] docker compose adds Redpanda (Kafka-compatible, simpler local setup)
[ ] API publishes to play-events on every stream start
[ ] Analytics worker consumes play-events, updates play_count in Postgres
[ ] Upload endpoint returns 202, worker processes async
[ ] Track has status field: pending | processing | ready | error
[ ] Retry logic with exponential backoff in worker on transient failures
[ ] Dead letter queue for permanently failed events
[ ] Observe: consumer lag in Grafana (are workers keeping up?)
```

### Interview question to answer before Phase 3

> *"Why did you choose async processing for audio uploads?"*
> *"What happens if a Kafka consumer crashes mid-processing? How do you avoid duplicate processing?"*

---

## Phase 3 — Kubernetes

**Goal:** Learn what changes when you move from Compose to k8s.
**Estimated time:** 1–2 weeks
**Concepts from Part 1:** Kubernetes, HPA, network policies

### What you add

```
minikube (local) or kind
Helm chart for Beatstream
HPA on API deployment
Ingress controller (Nginx)
Network policies (API can reach Postgres, workers cannot reach API)
```

### Milestones

```
[ ] kubectl apply -f manifests/ brings up full stack
[ ] HPA: watch pod count increase during k6 stress test
[ ] Rolling update: kubectl set image + watch zero-downtime
[ ] Rollback: kubectl rollout undo
[ ] Network policy: prove worker pods cannot reach API pods
[ ] Liveness probe kills deadlocked pod, readiness probe stops traffic during startup
[ ] Resource limits set on all pods (no single pod can hog node)
[ ] kubectl top pods shows realistic CPU/memory usage
```

### Interview question to answer before Phase 4

> *"How does HPA decide when to scale? What are its limitations?"*
> *"What's the difference between liveness and readiness probes?"*

---

## Phase 4 — Cloud deployment

**Goal:** Deploy to real AWS. Feel the real latency, real costs.
**Estimated time:** 2 weeks
**Concepts from Part 1:** IaC, CDN, multi-AZ

### What you build

```
infra/terraform/
  ├── vpc/          VPC, subnets (public/private), NAT gateway
  ├── alb/          Application Load Balancer, target groups, ACM cert
  ├── ecs/          ECS Fargate cluster, task definitions, service
  ├── rds/          Aurora PostgreSQL Serverless v2 (scales to zero)
  ├── elasticache/  Redis cluster
  ├── s3/           Audio storage bucket, lifecycle rules
  ├── cloudfront/   CDN in front of S3 and ALB
  └── msk/          Managed Kafka (MSK Serverless)
```

### Milestones

```
[ ] terraform plan shows no errors before any apply
[ ] terraform apply creates full stack in ~15 minutes
[ ] HTTPS endpoint live (ACM cert + ALB listener)
[ ] Audio file served via CloudFront PoP, not direct from S3
[ ] RDS in private subnet (not reachable from internet)
[ ] Simulate AZ failure: terminate all ECS tasks in one AZ, ALB routes around it
[ ] Measure and document real p99 latency from Taiwan to ap-northeast-1
[ ] Cost estimate: what does this stack cost per month at 10K DAU?
[ ] terraform destroy tears down cleanly (no orphaned resources)
```

### Interview question to answer before Phase 5

> *"Walk me through your AWS architecture. Why did you choose ECS over EKS?"*
> *"What happens when an availability zone goes down?"*

---

## Phase 5 — Observability

**Goal:** You can't improve what you can't measure. Add full-stack observability.
**Estimated time:** 1 week
**Concepts from Part 1:** Metrics, logs, traces, SLOs

### What you add

```
OpenTelemetry SDK in API (traces + metrics)
Jaeger for trace visualization
CloudWatch for logs (in AWS)
Grafana dashboard for SLO burn rate
PagerDuty alert when error budget burns > 5%/hour
```

### SLO dashboard to build

```
Panel 1: Availability (7d rolling)       → should show > 99.9%
Panel 2: p99 stream latency              → should show < 500ms
Panel 3: p99 search latency             → should show < 200ms
Panel 4: Error budget remaining (30d)   → warning at < 25%
Panel 5: Request rate by endpoint       → traffic pattern visibility
Panel 6: Kafka consumer lag             → worker health
Panel 7: Cache hit rate                 → Redis effectiveness
Panel 8: DB connection pool utilization → saturation warning
```

### Milestones

```
[ ] Every API request has a trace_id, propagated through all calls
[ ] Jaeger shows full request breakdown including DB + Redis spans
[ ] Find and fix one slow query by reading the trace
[ ] SLO dashboard shows real burn rate
[ ] Alert fires when error rate > 1% for 5 minutes
[ ] Runbook written: "p99 latency alert fired — what to check first"
[ ] Load test with observability on: watch metrics, traces, logs in real-time
```

---

## Final system check

Before calling it done, you should be able to answer every question below
from memory, not from notes — because you've debugged the real thing.

**API design**
- Why cursor pagination instead of offset?
- How do you make POST idempotent?
- When do you return 202 vs 200 vs 201?

**Database**
- Why is the `play_events` table partitioned?
- What index type did you use for full-text search and why?
- How does PgBouncer help with connection pool exhaustion?

**Caching**
- What's the difference between cache-aside and write-through?
- How do you handle a cache stampede on a popular track?
- What eviction policy did you choose and why?

**Load balancing**
- Why least_conn instead of round-robin for this service?
- What happens when an upstream fails a health check mid-request?
- How do you do a zero-downtime deployment with Nginx?

**Rate limiting**
- Why is a Lua script required for the Redis token bucket?
- What's the difference between per-user and per-IP rate limiting?

**Kafka**
- What happens if a consumer crashes after reading but before committing?
- Why did you partition play-events by user_id?
- How do you handle poison pills (events that always fail)?

**Kubernetes**
- What's the difference between a liveness and readiness probe?
- Why does HPA wait 5 minutes before scaling down?
- How does a rolling update guarantee zero downtime?

**Observability**
- What are the four golden signals?
- How do you use error budget to make deployment decisions?
- A p99 latency alert fired at 3am. What are the first three things you check?

---

# Classic interview questions

| Question | Core concepts | Phase where you built it |
|----------|--------------|--------------------------|
| Design Spotify / Netflix | CDN, streaming, search, recommendations | Beatstream (all phases) |
| Design a URL shortener | Hashing, redirect, analytics | interview-prep/url-shortener/ |
| Design a rate limiter | Token bucket, Redis, distributed | Phase 1 |
| Design Twitter timeline | Fan-out, cache, eventual consistency | interview-prep/twitter-timeline/ |
| Design YouTube | Video transcoding, CDN, view counts | Beatstream analog (audio) |
| Design a notification system | Fan-out, queues, push/email | Phase 2 (play events) |
| Design a distributed cache | Consistent hashing, eviction, replication | Phase 1 |
| Design a message queue | Kafka internals, ordering, delivery | Phase 2 |
| Design an API gateway | Rate limiting, auth, routing, circuit breaker | Phase 1 |

---

# Learning journal

Every time you implement something and it breaks, write it down.

See `journal/template.md` for the format.

```markdown
## YYYY-MM-DD — [Phase X] [Topic]

### What I ran
One paragraph. Specific commands, not vague summaries.

### What broke
The exact error message. Your first wrong hypothesis.

### How I found the root cause
Commands used. Logs read. Metrics checked.

### Root cause
The real explanation, tied to a concept from Part 1.

### What I'd say in an interview
"I actually built this and ran into X. The root cause was Y,
 which is why in production you need to..."

### Open questions
Things this raised that you don't understand yet.
```

---

## References

**Books**
- *Designing Data-Intensive Applications* — Martin Kleppmann ← read this first, above everything
- *System Design Interview Vol 1 & 2* — Alex Xu
- *The Google SRE Book* — free at sre.google

**Papers worth reading**
- Dynamo: Amazon's Highly Available Key-value Store (2007)
- Kafka: a Distributed Messaging System for Log Processing (2011)
- Spanner: Google's Globally Distributed Database (2012)

**Tools documentation**
- [k6 docs](https://k6.io/docs/) — load testing
- [Prometheus querying](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Terraform AWS provider](https://registry.terraform.io/providers/hashicorp/aws/latest/docs)
- [pgvector](https://github.com/pgvector/pgvector) — vector similarity in Postgres

---

*The goal of this repo is not to read it. It's to make every commit a real thing you ran.*
*If you've never seen a cascading failure in Grafana at 2am, you don't know distributed systems yet.*
*This repo is how you get there safely, on your own terms.*
