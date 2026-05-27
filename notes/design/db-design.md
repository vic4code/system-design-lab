# DB Design Philosophy & Selection Logic

> Cross-cutting reference — applies to all phases.
> This is the "why" behind every database decision in Beatstream.

---

## Core principle: use boring technology

> "Choose the most boring database that solves your problem."

Boring means: widely understood, battle-tested, rich ecosystem, easy to hire for.
Only upgrade when you have a concrete problem that boring doesn't solve.

---

## The selection hierarchy

```
Start here:
    PostgreSQL — can it do this?
        ├── YES → use PostgreSQL
        └── NO or "not efficiently" →
              Does it need sub-millisecond latency and simple lookups?
                  ├── YES → Redis
                  └── NO →
                        Is it time-series data (millions of events/sec)?
                            ├── YES → TimescaleDB or ClickHouse
                            └── NO →
                                  Is it flexible schema / document-oriented?
                                      ├── YES → DynamoDB or MongoDB
                                      └── NO → re-examine the problem
```

---

## Why PostgreSQL is the default

| Capability | PostgreSQL | MySQL | SQLite |
|---|---|---|---|
| Full-text search | ✅ built-in (`tsvector`) | ⚠️ limited | ❌ |
| JSON / JSONB | ✅ queryable, indexed | ⚠️ JSON only | ❌ |
| Window functions | ✅ | ✅ | ⚠️ limited |
| CTEs, recursive | ✅ | ✅ 8.0+ | ✅ |
| LISTEN/NOTIFY | ✅ (lightweight pub/sub) | ❌ | ❌ |
| Partial indexes | ✅ | ❌ | ✅ |
| Row-level security | ✅ | ❌ | ❌ |
| PostGIS (geo) | ✅ | ⚠️ | ❌ |

**Beatstream uses:**
- `tsvector` for track/artist full-text search (Phase 0)
- `gen_random_uuid()` for UUIDs (no application-level ID generation)
- `TIMESTAMPTZ` everywhere (always UTC, auto-converts on read)
- `GENERATED ALWAYS AS (...)` for search vector (no application sync needed)
- `ON DELETE CASCADE` / `SET NULL` for referential integrity

---

## When to use Redis (and when NOT to)

### Use Redis for:
| Use case | Beatstream example | Data type |
|---|---|---|
| Cache hot queries | `GET /v1/tracks` → `cache:tracks:list` | String (JSON) |
| Rate limiting | Login brute-force counter | String (INCR) |
| Token bucket | Stream rate limiter | Hash (HMGET/HMSET) |
| Session store | (future: if we switch from JWT) | Hash |
| Pub/sub (lightweight) | Track status updates | Pub/Sub |
| Leaderboard / rankings | Play count top-10 | Sorted Set (ZADD) |

### Do NOT use Redis for:
- **Primary data store** — Redis is volatile. A restart without AOF/RDB = data loss. PostgreSQL is the source of truth.
- **Complex queries** — No joins, no aggregations, no transactions. If you're writing Lua scripts to simulate a JOIN, you've already made a mistake.
- **Large blobs** — Audio files go to S3. Redis is for small, frequently-read values.

### Cache invalidation rule
```
Write path:   INSERT/UPDATE PostgreSQL → DELETE Redis key
Read path:    GET Redis → (miss) → SELECT PostgreSQL → SET Redis (TTL 5min)
```

Never cache stale or non-final data. In Beatstream:
```go
// WRONG: don't cache tracks with status != "ready"
// A track being processed is NOT stable data

// RIGHT: only cache after status == "ready"
if track.Status == "ready" {
    rdb.Set(ctx, "track:"+id, json, 5*time.Minute)
}
```

---

## Schema design principles

### 1. Normalise first, denormalise when measured

Start with 3NF (no redundancy). Only denormalise when:
- You have a query that's genuinely slow (measured, not guessed)
- You've exhausted index options
- The denormalised data has a clear owner and sync strategy

### 2. UUIDs vs sequential IDs

| | UUID (v4) | BIGSERIAL |
|---|---|---|
| Global uniqueness | ✅ (merge, shard-safe) | ❌ (per-table) |
| Index performance | ⚠️ random inserts = page splits | ✅ append-only |
| Expose in URLs | ✅ (opaque, safe) | ❌ (leaks row count) |
| Size | 16 bytes | 8 bytes |

**Beatstream uses UUIDs** — distributed system, no sequential IDs leaked to users.
At scale with write-heavy tables, consider **UUIDv7** (time-ordered, avoids page splits).

### 3. Index strategy

```sql
-- Cover the query, not just the WHERE clause
-- Bad: index only on user_id, but query also selects created_at
CREATE INDEX play_events_user_idx ON play_events(user_id);

-- Good: covering index — no heap fetch needed
CREATE INDEX play_events_user_cover_idx
    ON play_events(user_id, played_at DESC)
    INCLUDE (track_id, ms_played);

-- Partial index — index only the relevant subset
CREATE INDEX tracks_pending_idx ON tracks(created_at)
    WHERE status = 'pending';  -- Only index unprocessed tracks
```

### 4. Timestamptz everywhere

```sql
-- Always TIMESTAMPTZ, never TIMESTAMP
-- TIMESTAMP stores no timezone → ambiguous on DST boundaries
created_at TIMESTAMPTZ NOT NULL DEFAULT now()
```

### 5. Soft delete vs hard delete

| | Soft delete (`deleted_at`) | Hard delete |
|---|---|---|
| GDPR Art. 17 | ✗ (data still exists) | ✓ |
| Audit trail | ✅ | Need separate audit log |
| Query complexity | ❌ (must filter `WHERE deleted_at IS NULL`) | ✅ |
| Foreign keys | ❌ (FK to deleted rows is confusing) | ✅ |

**Beatstream uses hard delete + audit_logs table.**
- GDPR erasure: hard delete the row
- Audit trail: the `audit_logs` FK becomes NULL (`ON DELETE SET NULL`) — preserves security record without PII

---

## AWS DB selection mapping

| Workload | Local (Beatstream) | AWS Option | Why |
|---|---|---|---|
| Primary relational | PostgreSQL | Aurora PostgreSQL Serverless v2 | Auto-scale, HA multi-AZ, compatible |
| Cache / rate limit | Redis (docker) | ElastiCache Serverless | Sub-ms, managed, auto-scale |
| Stream/event store | Redpanda | MSK Serverless | Kafka-compatible, no broker management |
| Object storage | MinIO | S3 | Industry standard, 11 nines durability |
| Analytics (future) | – | Redshift / Athena on S3 | Columnar, pay-per-query |
| Full-text search (scale) | pg `tsvector` | OpenSearch | When PG full-text isn't enough (fuzzy, ML ranking) |

**Aurora Serverless v2 vs RDS PostgreSQL:**
- Serverless: auto-scales ACUs in ~1s, 0.5 ACU minimum (~$0.10/hr at idle)
- RDS: fixed instance size, manual resize = downtime, predictable cost at steady load
- Rule of thumb: use Serverless if traffic is spiky or unpredictable; use RDS if load is steady and you want to optimise cost

---

## Connection pooling

PostgreSQL has a hard limit on connections (~100-300 depending on instance).
Each Go goroutine that runs a query needs a connection.

```
3 ECS tasks × 20 conns each = 60 conns
+ worker × 10 conns = 10 conns
Total: 70 conns  →  fine for t3.medium (max 170)

At scale (20 ECS tasks):
20 × 20 = 400 conns  →  exceeds limit
```

**Solution:** PgBouncer (sidecar) or RDS Proxy
- PgBouncer: open source, transaction pooling, 1000s of app conns → ~20 DB conns
- RDS Proxy: managed, IAM auth, automatic failover, ~$0.015/conn/hr

In Beatstream (Phase 0–7): `pgxpool` with `MaxConns=20` per replica is enough.
Phase 8 Terraform: add RDS Proxy if we exceed 3 ECS tasks.

---

## Interview questions

> *"Why PostgreSQL over DynamoDB for Beatstream?"*
> Beatstream has complex relational queries: tracks JOIN artists, playlists JOIN tracks with ordering, full-text search. DynamoDB excels at single-table access patterns with a known partition key — it would require significant denormalisation and multiple tables for our access patterns. We'd lose JOIN capabilities and full-text search. PostgreSQL handles all our current needs, and Aurora Serverless auto-scales to handle traffic spikes.

> *"When would you switch from pg full-text search to OpenSearch?"*
> When we need: fuzzy matching ("radiohed" → Radiohead), field boosting per-result-click (ML ranking), multi-language analysers, or query volume exceeding ~1000 search RPS. At that scale, putting search load on the primary database risks affecting write throughput.

> *"How do you handle database migrations in production without downtime?"*
> Two principles: (1) expand-then-contract — add the new column nullable first, backfill, add NOT NULL constraint later; (2) never drop a column in the same deploy that removes the code reading it. Our current inline migration (`db.Migrate()`) is fine for dev; production uses `golang-migrate` with a separate step before ECS deployment (blue-green: migrate → deploy new tasks → drain old).

> *"What's your strategy for slow query investigation?"*
> 1. `EXPLAIN (ANALYZE, BUFFERS)` on the slow query. 2. Check for seq scans on large tables — add index. 3. Check for N+1 queries in application code — use batch fetch or JOIN. 4. Check index bloat with `pg_stat_user_indexes`. 5. Enable `auto_explain` in RDS for queries over 1s. CloudWatch `DatabaseConnections` + `DBLoad` metrics alert before it becomes a crisis.
