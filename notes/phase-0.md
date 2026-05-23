# Phase 0 — Local Monolith

## What we built

A minimal Spotify-like API running as a single process with Postgres, MinIO (S3-compatible), and Redis — all in Docker Compose.

Endpoints: `POST /artists`, `GET /artists/:id`, `POST /tracks`, `GET /tracks/:id`, `GET /tracks/:id/stream`, playlist CRUD, `GET /search`.

---

## Key concepts

### Pre-signed URLs for audio streaming

The stream endpoint does **not** proxy the audio file through the API server. Instead:

1. Client calls `GET /tracks/:id/stream`
2. API looks up the `audio_key` in Postgres
3. API calls MinIO to generate a pre-signed URL (valid 1 hour)
4. API returns `307 Temporary Redirect` to the pre-signed URL
5. Client fetches audio **directly from MinIO/S3**

**Why:** If the API proxied audio, a single 5MB file × 1000 concurrent listeners = 5GB/s of bandwidth through the API server. Pre-signed URLs offload that entirely to object storage.

### Play events as a separate table

`play_events` is its own table, not just a counter on `tracks`.

```sql
CREATE TABLE play_events (
  id         UUID DEFAULT gen_random_uuid() PRIMARY KEY,
  track_id   UUID REFERENCES tracks(id),
  played_at  TIMESTAMPTZ DEFAULT now(),
  source     TEXT
);
```

**Why:** A counter tells you total plays. A table tells you *when* people listened, *from where*, and lets you partition by time for analytics. You can answer "plays in the last 7 days" or "peak listening hour" — a counter can't.

The `play_count` on `tracks` is a denormalized cache of the count — fast to read, updated asynchronously (Phase 2 makes this proper).

### Full-text search with PostgreSQL tsvector

```sql
WHERE search_vector @@ plainto_tsquery('english', $1)
ORDER BY ts_rank(search_vector, plainto_tsquery('english', $1)) DESC
```

`search_vector` is a generated column updated automatically on insert/update. No external search service needed at this scale.

### Graceful shutdown

```go
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit
srv.Shutdown(ctx) // waits for in-flight requests to finish
```

Without this, killing the process mid-request drops responses. Docker and Kubernetes send SIGTERM before SIGKILL — you have a window to finish cleanly.

---

## Gotchas discovered

- MinIO pre-signed URLs use the **internal** Docker hostname (`minio:9000`) by default. If the client is outside Docker, it gets a URL it can't reach. Need to configure the external URL for production.
- `play_count` increment is fire-and-forget in a goroutine — this is intentionally non-critical in Phase 0 but will be replaced by Kafka in Phase 2.

---

## Interview talking points

**"Walk me through your database schema."**
> tracks references artists, play_events references tracks. I separated play events from the counter on tracks because a separate table lets me query time-series data — total plays, plays per day, peak hour. The counter on tracks is a denormalized read cache.

**"How does pre-signed URL streaming work? Why not proxy through your API?"**
> The API generates a time-limited URL signed with the object storage credentials and redirects the client there. The client streams directly from S3/MinIO. Proxying would funnel all audio bandwidth through the API — at any real scale that's the first thing to saturate.
