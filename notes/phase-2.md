# Phase 2 — Async & Queues

## Architecture

```mermaid
graph TD
    Client(["Client"])

    subgraph Docker Compose
        NGINX["nginx :80"]

        subgraph "API Layer (×3)"
            API1["api-1"]
            API2["api-2"]
            API3["api-3"]
        end

        REDIS["Redis\ncache + rate limit"]
        PG[("PostgreSQL\ntracks / play_events")]
        MINIO["MinIO\naudio files"]
        RP["Redpanda :9092\nKafka-compatible"]

        subgraph "Worker"
            UW["Upload Worker\ntrack.uploads consumer"]
            AW["Analytics Worker\nplay.events consumer"]
        end
    end

    Client -->|"POST /tracks"| NGINX
    NGINX --> API1 & API2 & API3
    API1 -->|"1 upload file"| MINIO
    API1 -->|"2 INSERT status=pending"| PG
    API1 -->|"3 publish track.uploads"| RP
    API1 -->|"4 202 Accepted"| Client

    UW -->|"consume track.uploads"| RP
    UW -->|"UPDATE status=ready"| PG

    Client -->|"GET /tracks/:id/stream"| NGINX
    API2 -->|"publish play.events"| RP
    AW -->|"consume play.events"| RP
    AW -->|"INSERT play_events\nUPDATE play_count"| PG

    style NGINX fill:#009639,color:#fff
    style API1 fill:#4f86c6,color:#fff
    style API2 fill:#4f86c6,color:#fff
    style API3 fill:#4f86c6,color:#fff
    style REDIS fill:#d82c20,color:#fff
    style PG fill:#336791,color:#fff
    style MINIO fill:#c72c41,color:#fff
    style RP fill:#7b61ff,color:#fff
    style UW fill:#2e7d32,color:#fff
    style AW fill:#2e7d32,color:#fff
```

## Goal

Decouple two latency-sensitive paths from the synchronous request cycle:

1. **Upload processing** — transcoding audio is CPU-heavy and can take seconds; the HTTP response should not block on it.
2. **Play event recording** — at high throughput, writing every play directly to Postgres creates a write hotspot on the `tracks` table.

Redpanda (Kafka-compatible, single-binary, no ZooKeeper) is added as the message broker.

---

## What changed

### New services

| Service | Image | Role |
|---|---|---|
| `redpanda` | redpandadata/redpanda | Kafka-compatible broker, topics auto-created |
| `worker` | beatstream (target: worker) | Runs upload + analytics consumers |

### Topics

| Topic | Producer | Consumer group | Semantics |
|---|---|---|---|
| `track.uploads` | `POST /tracks` | `upload-workers` | at-least-once |
| `play.events` | `GET /tracks/:id/stream` | `analytics-workers` | at-least-once |

### Files added

```
beatstream/
├── internal/queue/kafka.go          # Producer + Consumer wrappers (franz-go)
├── internal/worker/upload.go        # Transcoding worker: pending → processing → ready
├── internal/worker/analytics.go     # Analytics worker: insert play_events + increment play_count
├── cmd/worker/main.go               # Worker binary entrypoint (runs both workers)
└── db/migrations/004_add_track_status.sql
```

### Files modified

```
├── internal/db/postgres.go          # + migration 004 (status column)
├── internal/handler/tracks.go       # POST /tracks → 202, publishes to track.uploads
│                                    # GET /tracks/:id/stream → publishes to play.events
├── cmd/api/main.go                  # + Kafka producer init
├── docker-compose.yml               # + redpanda, + worker service
├── Dockerfile                       # multi-target build (api, worker)
└── Makefile                         # + run-worker target
```

---

## Upload flow (before vs after)

**Before (Phase 1)**
```
POST /tracks
  → upload to MinIO (sync)
  → INSERT tracks (sync)
  → 201 Created (track ready immediately)
```

**After (Phase 2)**
```
POST /tracks
  → upload to MinIO (sync)           ← file must exist before worker runs
  → INSERT tracks status='pending'
  → publish to track.uploads
  → 202 Accepted {id, status: "pending"}

[upload worker]
  ← consume track.uploads
  → UPDATE status='processing'       ← optimistic lock prevents double-processing
  → sleep 2s (simulate ffmpeg)
  → UPDATE status='ready', duration_ms=N
```

Clients poll `GET /tracks/:id` until `status == "ready"`.

---

## At-least-once delivery and idempotency

franz-go with `kgo.AutoCommitMarks()` only commits offsets for records explicitly marked via `MarkCommitRecords`. If the worker crashes after processing but before marking, the record is redelivered.

**Upload worker idempotency guard:**
```go
// Check status before processing
if status != "pending" {
    return nil // already processed — safe to skip
}
// CAS-style update: only transitions from pending
UPDATE tracks SET status = 'processing' WHERE id = $1 AND status = 'pending'
```

This means even if the event is delivered twice, the second delivery is a no-op.

**Analytics worker:** duplicates result in a slightly over-counted `play_count`. Acceptable for analytics. For billing accuracy you would need event deduplication (e.g., include a UUID in the Kafka message and check a `processed_events` table).

---

## Why 202 instead of 200/201?

| Code | Meaning | When to use |
|---|---|---|
| `200 OK` | request fully satisfied | synchronous read |
| `201 Created` | resource created now | synchronous write |
| `202 Accepted` | queued for processing | async work |

Returning 201 would imply the track is ready to stream — but `duration_ms` is 0 and the audio hasn't been transcoded yet. 202 + status field makes the async contract explicit.

---

## Why Redpanda over Kafka?

| | Kafka | Redpanda |
|---|---|---|
| Dependencies | JVM + ZooKeeper (or KRaft) | Single Go binary |
| Local dev setup | Complex | `docker run` |
| Kafka protocol compatible | Yes (it's Kafka) | Yes |
| Production use | Yes | Yes |

For local dev and interviews: Redpanda dramatically reduces setup friction.

---

## Interview questions

**Q: Why use async processing for audio uploads? When do you return 202 vs 200?**

Audio transcoding is CPU/time-intensive (seconds to minutes). Blocking the HTTP connection while transcoding wastes server resources and degrades perceived performance. You return 202 when the work is *accepted but not yet complete* — the client can poll for status. Return 200/201 only when the work is fully done synchronously.

**Q: A Kafka consumer crashes mid-processing and restarts. How do you avoid processing the same event twice?**

Two parts: (1) delivery guarantee and (2) idempotent handler.

Franz-go's `AutoCommitMarks` gives at-least-once — records are redelivered after a crash. The handler is made idempotent: before processing, check if the track is still `pending`; use a conditional `UPDATE ... WHERE status = 'pending'` so only one consumer can claim the work. A second attempt sees `processing` or `ready` and skips.

For analytics (play events), duplicates are tolerable. For financial/critical data, include a UUID in the event payload and insert into a `processed_events` deduplication table inside the same DB transaction.

**Q: What's the problem with publishing to Kafka after the DB insert in the same HTTP handler?**

Split-brain risk: the DB write succeeds but the Kafka publish fails — the track is stuck `pending` forever. The correct fix is the **transactional outbox pattern**: write the event to an `outbox` table in the same DB transaction as the track insert, then have a separate relay process poll the outbox and publish to Kafka. This makes the DB the single source of truth and the publish eventually consistent but reliable.

---

## Demo

**前置：** `make up && make migrate && make seed`（包含 Redpanda）

---

### 1. 上傳 track → 看到 202 Accepted + status=pending（非同步）

```bash
# 先準備一個假的 mp3
dd if=/dev/urandom bs=1024 count=50 > /tmp/demo.mp3

# 需要先有 JWT token（Phase 5），但在 Phase 2 這個 endpoint 是 public
curl -s -X POST https://localhost/v1/tracks \
  -F "title=Async Demo" \
  -F "artist_id=11111111-1111-1111-1111-111111111111" \
  -F "duration_ms=180000" \
  -F "audio=@/tmp/demo.mp3;type=audio/mpeg" | python3 -m json.tool
```

**你應該看到：**
```json
{
  "id": "xxxx-...",
  "title": "Async Demo",
  "status": "pending",   ← 重點：不是 ready，是 pending
  "duration_ms": 0       ← 還沒被 worker 處理，duration 還是 0
}
```

**這說明了什麼：** API 回 202 不等 transcoding 完成。HTTP handler 做的事：① upload 到 MinIO ② INSERT status=pending ③ publish 到 Kafka `track.uploads` topic ④ 回 202。

---

### 2. 等 worker 處理 → 看到 status 從 pending 變 ready

```bash
TRACK_ID="（上面拿到的 id）"

# 馬上查：pending
curl -s https://localhost/v1/tracks/$TRACK_ID | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])"

# 等 2 秒，worker 從 Kafka 消費並處理
sleep 2

# 再查：應該是 ready 了
curl -s https://localhost/v1/tracks/$TRACK_ID | python3 -m json.tool | grep -E "status|duration"
```

**你應該看到：** `"status": "ready"` 和一個非 0 的 `"duration_ms"`。

**直接看 worker log 確認：**
```bash
docker compose logs worker --tail 10
```

**你應該看到：**
```
upload worker: transcoding track xxxx-...
upload worker: track xxxx-... ready (159909ms)
```

---

### 3. Redpanda topics — 看到兩個 topic 存在

```bash
docker exec beatstream-redpanda-1 rpk topic list
```

**你應該看到：**
```
NAME           PARTITIONS  REPLICAS
play.events    1           1
track.uploads  1           1
```

**觸發 play event，看 analytics worker 記錄：**
```bash
# 打 stream endpoint 觸發 play event
curl -sk https://localhost/v1/tracks/aaaa0001-0000-0000-0000-000000000000/stream -o /dev/null

# 看 worker 有沒有消費到
docker compose logs worker --tail 5
```

**你應該看到：**
```
analytics worker: recorded play for track aaaa0001-...
```

---

### 4. 驗證 Kafka 解耦效果 — 停掉 worker，upload 還是成功

```bash
# 停掉 worker
docker compose stop worker

# 上傳一個 track
curl -s -X POST https://localhost/v1/tracks \
  -F "title=No Worker Test" \
  -F "artist_id=11111111-1111-1111-1111-111111111111" \
  -F "audio=@/tmp/demo.mp3;type=audio/mpeg" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['id'], d['status'])"
```

**你應該看到：** 仍然回傳一個 id 和 `pending` — API 不依賴 worker 存活，訊息在 Kafka 裡等著。

```bash
# 重啟 worker，它會消費積壓的訊息
docker compose start worker
sleep 3
docker compose logs worker --tail 5
# 看到 "track xxxx ready" — 補處理完了
```

**這說明了什麼：** Kafka 是 durable message log，不是 in-memory queue。Worker 掛掉再重啟，不會漏訊息。
