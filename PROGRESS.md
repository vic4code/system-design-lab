# Beatstream — Progress Tracker

## Currently on Phase 3 ⬜

---

## Phase 0 — Local Monolith ✅
> Goal: get the service running — upload, stream, search

**Files added:**
```
beatstream/
├── cmd/api/main.go                  # entrypoint: routing, graceful shutdown
├── internal/db/postgres.go          # PostgreSQL connection + migrations
├── internal/storage/s3.go           # MinIO (S3-compatible) upload / pre-signed URL
├── internal/handler/tracks.go       # GET /tracks/:id, POST /tracks, GET /tracks/:id/stream
├── internal/handler/artists.go      # GET /artists/:id, POST /artists
├── internal/handler/playlists.go    # playlist CRUD
├── internal/handler/search.go       # GET /search (PostgreSQL full-text)
├── internal/middleware/requestid.go # X-Request-ID header
├── db/migrations/                   # 001 tracks, 002 playlists, 003 play_events, seed
├── Dockerfile                       # multi-stage build (golang → alpine)
└── docker-compose.yml               # postgres, redis, minio, prometheus, grafana
```

**Key questions you should be able to answer:**
- Why use a pre-signed URL instead of proxying audio through the API server?
- Why is `play_events` a separate table instead of just a counter on `tracks`?

---

## Phase 1 — Load Balancing & Caching ✅
> Goal: put the system under load, find the bottleneck, fix it with caching and rate limiting

**Files added:**
```
beatstream/
├── nginx/nginx.conf                           # upstream × 3, least_conn, health checks
├── internal/cache/redis.go                    # Redis client wrapper
├── internal/metrics/metrics.go                # Prometheus counters / histograms
├── internal/middleware/ratelimit.go            # token bucket rate limiter (Redis Lua script)
├── internal/middleware/prometheus.go           # records latency + count per request
├── grafana/provisioning/datasources/          # auto-provisions Prometheus datasource
└── grafana/provisioning/dashboards/           # Beatstream Phase 1 dashboard (6 panels)
```

**Files modified:**
```
├── internal/handler/tracks.go   # + cache-aside on GET /tracks/:id (1h TTL)
├── internal/handler/search.go   # + cache search results (1min TTL)
├── cmd/api/main.go               # + Redis connection, rate limiter on /stream, metrics server
└── prometheus.yml                # + explicit /metrics scrape path
```

**Key questions you should be able to answer:**
- What is the difference between cache-aside and write-through?
- Why does the token bucket rate limiter use a Lua script instead of regular Redis commands?
- After adding the rate limiter, the k6 error rate jumped to 20%. Is that a bug?

**Experiments to run:**
```bash
# 1. Kill one API instance — does traffic automatically shift to the other two?
docker compose stop api-2
k6 run -e TRACK_ID=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa k6/load.js

# 2. Kill Redis — does the API keep serving? (fail-open design)
docker compose stop redis
curl http://localhost/v1/tracks/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
```

---

## Phase 2 — Async & Queues ✅
> Goal: decouple upload processing, handle play events at scale with Kafka

**Files added:**
```
beatstream/
├── internal/queue/kafka.go          # Producer + Consumer wrappers (franz-go, at-least-once)
├── internal/worker/upload.go        # Transcoding worker: pending → processing → ready
├── internal/worker/analytics.go     # Analytics worker: insert play_events + increment play_count
├── cmd/worker/main.go               # Worker binary (runs both workers as goroutines)
└── db/migrations/004_add_track_status.sql
```

**Files modified:**
```
├── internal/db/postgres.go          # + migration 004 (status column, default 'ready')
├── internal/handler/tracks.go       # POST /tracks → 202 Accepted, publishes to track.uploads
│                                    # GET /tracks/:id/stream → publishes to play.events
├── cmd/api/main.go                  # + Kafka producer init
├── docker-compose.yml               # + redpanda service, + worker service
├── Dockerfile                       # multi-target build (api target, worker target)
└── Makefile                         # + run-worker target
```

**Key questions you should be able to answer:**
- Why use async processing for audio uploads? When do you return 202 vs 200?
- A Kafka consumer crashes mid-processing and restarts. How do you avoid processing the same event twice?
- What is the transactional outbox pattern and why do you need it here?

**Experiments to run:**
```bash
# 1. Upload a track — check it starts as pending
curl -X POST http://localhost/v1/tracks -F title="Test" -F artist_id=<id> -F audio=@song.mp3
# → 202 {"status": "pending"}

# 2. Poll until ready (worker takes ~2s to "transcode")
curl http://localhost/v1/tracks/<id>
# → {"status": "ready", "duration_ms": ...}

# 3. Kill the worker mid-processing — restart it and verify the track ends up ready
docker compose stop worker && docker compose start worker
```

---

## Phase 3 — Kubernetes ✅
> Goal: replace Docker Compose with K8s manifests; add HPA, probes, ConfigMap/Secret, StatefulSets

**Files added:**
```
beatstream/k8s/
├── namespace.yaml             # beatstream namespace
├── configmap.yaml             # non-sensitive env config (endpoints, ports)
├── secret.yaml                # credentials (DATABASE_URL, MinIO, Redis)
├── api-deployment.yaml        # Deployment: 3 replicas, RollingUpdate, liveness + readiness probes
├── api-service.yaml           # ClusterIP Service (load-balances across API pods)
├── api-hpa.yaml               # HPA: 2–10 pods, CPU 60%, memory 70%
├── worker-deployment.yaml     # Deployment: 1 replica, Recreate strategy
├── postgres-statefulset.yaml  # StatefulSet + volumeClaimTemplate (5Gi PVC per pod)
├── postgres-service.yaml      # Headless service for stable pod DNS
├── redis-deployment.yaml      # Deployment + ClusterIP service
├── redpanda-statefulset.yaml  # StatefulSet + volumeClaimTemplate (10Gi) + headless service
├── minio-deployment.yaml      # Deployment + ClusterIP service
└── ingress.yaml               # Ingress (nginx-ingress): /v1/, /healthz, /ready
```

**Key questions you should be able to answer:**
- What is the difference between livenessProbe and readinessProbe? Why should liveness never check the DB?
- HPA is configured but pods aren't scaling. What would you check?
- Why does Postgres use a StatefulSet instead of a Deployment?
- Why does the worker use `strategy: Recreate` instead of RollingUpdate?

**Experiments to run:**
```bash
# 1. Stand up the cluster
make k8s-cluster
make k8s-load
make k8s-deploy

# 2. Watch pods and HPA
kubectl -n beatstream get pods,hpa -w

# 3. Trigger scale-out: stress-test with k6 (run in docker compose first for real load)
kubectl run -it --rm load --image=busybox -- \
  wget -q -O- http://api.beatstream.svc.cluster.local/healthz

# 4. Simulate pod crash and watch self-heal
kubectl -n beatstream delete pod -l app=api --field-selector=status.phase=Running

# 5. Rolling update (change image tag in api-deployment.yaml, then:)
kubectl apply -f k8s/api-deployment.yaml
kubectl -n beatstream rollout status deployment/api

# 6. Rollback
kubectl -n beatstream rollout undo deployment/api
```

---

## Phase 4 — Cloud Deployment ⬜
## Phase 5 — Observability ⬜
