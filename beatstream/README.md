# Beatstream

A Spotify-like audio streaming API built progressively across 8 phases — from a local monolith to a cloud-deployed, production-grade system on AWS. Each phase introduces one architectural concept and demonstrates it with working code and observable behaviour.

---

## Quick Start

**Prerequisites:** Docker, Docker Compose, Go 1.22+, make

```bash
# 1. Start all services (Postgres, Redis, MinIO, Redpanda, nginx, Jaeger, Grafana)
make up

# 2. Run all migrations (idempotent — safe to re-run)
make migrate

# 3. Load sample data (Radiohead, Portishead, Björk, Massive Attack, etc.)
make seed

# 4. Verify the stack is healthy
curl -s https://localhost/healthz    # → {"status":"ok"}
curl -s https://localhost/ready      # → {"status":"ready"}
curl -sk https://localhost/v1/tracks | python3 -m json.tool | head -20
```

> **Note on HTTPS:** nginx redirects HTTP → HTTPS. Use `curl -sk` (skip certificate verification) or set up mkcert:
> ```bash
> brew install mkcert && make certs-trust && make certs
> ```

### Optional: run the frontend

```bash
make web-install    # install npm dependencies (first time only)
make web-dev        # starts Next.js at http://localhost:3001
```

---

## Service Map

| Service | URL | Credentials |
|---------|-----|-------------|
| API (via nginx) | https://localhost | — |
| MinIO console | http://localhost:9001 | minioadmin / minioadmin |
| Grafana | http://localhost:3000 | admin / admin |
| Jaeger (traces) | http://localhost:16686 | — |
| Prometheus | http://localhost:9090 | — |
| Redpanda (Kafka) | localhost:9092 | — |
| Postgres | localhost:5432 | user / pass / beatstream |
| Redis | localhost:6379 | — |

---

## API Reference

All routes are prefixed with `/v1`.

### Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/auth/register` | — | Create account, returns JWT |
| `POST` | `/auth/login` | — | Login, returns JWT |
| `GET` | `/auth/me` | ✓ | Current user profile |
| `DELETE` | `/me` | ✓ | GDPR erasure — soft-delete account |
| `GET` | `/me/export` | ✓ | GDPR data export (profile + playlists) |

### Tracks

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/tracks` | — | List all tracks |
| `GET` | `/tracks/:id` | — | Get track metadata |
| `POST` | `/tracks` | ✓ | Upload track (multipart: title, artist_id, audio file) |
| `GET` | `/tracks/:id/stream` | — | Get pre-signed audio stream URL (307 redirect) |

### Artists

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/artists` | — | List artists |
| `GET` | `/artists/:id` | — | Get artist |
| `POST` | `/artists` | ✓ | Create artist |

### Playlists

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/playlists` | — | List playlists |
| `GET` | `/playlists/:id` | — | Get playlist |
| `POST` | `/playlists` | ✓ | Create playlist |
| `GET` | `/playlists/:id/tracks` | — | List tracks in playlist |
| `POST` | `/playlists/:id/tracks` | ✓ | Add track to playlist |
| `DELETE` | `/playlists/:id/tracks/:track_id` | ✓ | Remove track from playlist |

### Search

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/search?q=...` | — | Full-text search across tracks |

### Admin (role: admin required)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/admin/users` | admin | List all users |
| `DELETE` | `/admin/users/:id` | admin | Hard-delete a user |
| `GET` | `/admin/audit-logs` | admin | Query audit log |

---

## Phases

Each phase builds on the previous one. The doc for each phase describes the architecture, key design decisions, observable demo steps, and interview talking points.

| Phase | What it introduces | Doc |
|-------|--------------------|-----|
| [Phase 0](../docs/phase-0.md) | Local monolith — Postgres, MinIO, full-text search, pre-signed URLs | [→ doc](../docs/phase-0.md) |
| [Phase 1](../docs/phase-1.md) | Load balancing (nginx), Redis cache-aside, token-bucket rate limiting, Prometheus + Grafana | [→ doc](../docs/phase-1.md) |
| [Phase 2](../docs/phase-2.md) | Async pipelines — Kafka (Redpanda), upload worker, analytics worker, at-least-once delivery | [→ doc](../docs/phase-2.md) |
| [Phase 3](../docs/phase-3.md) | Kubernetes — Deployments, StatefulSets, HPA, rolling updates, secrets | [→ doc](../docs/phase-3.md) |
| [Phase 4](../docs/phase-4.md) | Next.js frontend, React Context, debounce, CORS, Vercel deployment | [→ doc](../docs/phase-4.md) |
| [Phase 5](../docs/phase-5.md) | JWT authentication, bcrypt, protected routes, multi-user | [→ doc](../docs/phase-5.md) |
| [Phase 6](../docs/phase-6.md) | Security — structured logging (zap), audit trail, security headers, rate limiting, TLS, OpenTelemetry | [→ doc](../docs/phase-6.md) |
| [Phase 7](../docs/phase-7.md) | RBAC (role column + JWT claim), GDPR erasure, data export, consent tracking | [→ doc](../docs/phase-7.md) |
| [Phase 8](../docs/phase-8.md) | AWS deployment — ECS Fargate, Aurora, ElastiCache, MSK, CloudFront, Terraform | [→ doc](../docs/phase-8.md) |

---

## Make Targets

```
make up            start all Docker Compose services
make down          stop all services
make migrate       run all DB migrations (001 → 007, idempotent)
make seed          load sample track/artist/playlist data
make build         compile API and worker binaries
make run           run API locally (services must be up)
make run-worker    run worker locally
make test          run Go tests
make load-test     k6 load test (requires TRACK_ID env var)
make fmt           gofmt
make lint          golangci-lint

make certs         generate local TLS certs via mkcert
make certs-trust   trust mkcert CA in system keychain (run once)

make web-install   npm install for Next.js frontend
make web-dev       start Next.js dev server at :3001

make k8s-cluster   create local kind cluster
make k8s-load      build + load images into kind
make k8s-deploy    apply all K8s manifests
make k8s-status    show pods, services, HPA
make k8s-delete    tear down kind cluster

make infra-init    terraform init
make infra-plan    terraform plan
make infra-apply   terraform apply (~15 min)
make infra-push    build + push images to ECR
make infra-deploy  force new ECS deployment
make infra-destroy terraform destroy
```

---

## Project Layout

```
beatstream/
├── cmd/
│   ├── api/          main.go — HTTP server entrypoint
│   └── worker/       main.go — Kafka consumer entrypoint
├── internal/
│   ├── cache/        Redis client wrapper
│   ├── db/           Postgres connection + migration runner
│   ├── handler/      HTTP handlers (auth, tracks, artists, playlists, admin)
│   ├── logger/       zap structured logger
│   ├── metrics/      Prometheus counters + histograms
│   ├── middleware/   JWT auth, RBAC, audit log, rate limit, security headers, CORS
│   ├── queue/        Kafka producer + consumer (franz-go)
│   ├── storage/      S3/MinIO client (AWS SDK v2)
│   ├── telemetry/    OpenTelemetry tracer setup
│   └── worker/       upload + analytics consumer logic
├── db/
│   └── migrations/   001–007 SQL migration files
├── k8s/              Kubernetes manifests (Phase 3)
├── infra/terraform/  AWS infrastructure (Phase 8)
├── web/              Next.js frontend (Phase 4)
├── k6/               load test scripts
├── grafana/          Grafana dashboard provisioning
├── nginx/            nginx config + TLS certs
├── docker-compose.yml
└── Makefile
```
