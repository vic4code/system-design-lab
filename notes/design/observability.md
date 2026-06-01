# Observability, SLA & DevOps

> Cross-cutting reference — applies to Phase 6 (implementation) and Phase 8 (cloud).

---

## The three pillars of observability

```
         ┌─────────┐   ┌─────────┐   ┌─────────┐
         │  LOGS   │   │ METRICS │   │ TRACES  │
         └────┬────┘   └────┬────┘   └────┬────┘
              │             │             │
         "What happened"  "How often /  "Where did
          to this request  how fast"     time go"
              │             │             │
         zap + audit_logs  Prometheus    OpenTelemetry
              │             │             │
         CloudWatch Logs  CloudWatch     Jaeger /
         Logs Insights    Metrics        X-Ray
```

All three must be correlated by **trace_id / request_id**.
Without correlation, you have three islands of data instead of one story.

---

## Logs (Phase 6 ✅)

### What we have

zap structured JSON — every request line:
```json
{
  "level": "info",
  "ts": "2026-05-27T14:00:00.123Z",
  "service": "beatstream-api",
  "msg": "request",
  "status": 200,
  "method": "POST",
  "path": "/v1/playlists",
  "latency_ms": 12,
  "ip": "172.18.0.1",
  "request_id": "abc-123",
  "user_id": "uuid-here"
}
```

Log levels:
- `ERROR` — 5xx responses, panics, dependency failures
- `WARN`  — 4xx responses (client errors, rate limits)
- `INFO`  — normal requests, startup, shutdown
- `DEBUG` — verbose SQL, cache hits/misses (disabled in prod)

### AWS CloudWatch setup (Phase 8)

```hcl
# ECS task definition — send stdout to CloudWatch
log_configuration = {
  logDriver = "awslogs"
  options = {
    awslogs-group         = "/beatstream/api"
    awslogs-region        = "ap-northeast-1"
    awslogs-stream-prefix = "ecs"
  }
}
```

**CloudWatch Logs Insights queries:**
```sql
-- p99 latency by endpoint (last 1 hour)
fields path, latency_ms
| stats pct(latency_ms, 99) as p99 by path
| sort p99 desc

-- Error rate by minute
fields status
| filter status >= 500
| stats count() as errors by bin(1m)

-- Login failures by IP (brute force detection)
fields ip, action, status_code
| filter action = "auth.login" and status_code = 401
| stats count() as failures by ip
| sort failures desc
```

**Log retention:** 90 days (GDPR: delete personal data including IP addresses after 90 days)

---

## Metrics (Phase 0 ✅ — Prometheus + Grafana)

### What we have

Prometheus metrics exposed at `:9090/metrics`:
- `http_requests_total` (counter: method, path, status)
- `http_request_duration_seconds` (histogram: p50/p95/p99 latency)
- `rate_limit_decisions_total` (counter: allowed/denied)
- Go runtime metrics (goroutines, GC, memory)

### SLI/SLO/SLA definitions

| Term | Definition | Example |
|---|---|---|
| **SLI** (Service Level Indicator) | The metric you measure | Request success rate |
| **SLO** (Service Level Objective) | The target you set internally | 99.9% success rate |
| **SLA** (Service Level Agreement) | The contract with users | 99.5% uptime, or credit |
| **Error budget** | How much failure is allowed | 0.1% × 30 days = 43 min/month |

### Beatstream SLOs

| Service | SLI | SLO | Measurement window |
|---|---|---|---|
| API availability | `(total_requests - 5xx) / total_requests` | 99.9% | rolling 30 days |
| API latency (p99) | `http_request_duration_seconds p99` | < 500ms | rolling 1 hour |
| Stream latency (p95) | Time to first audio byte | < 2s | rolling 1 hour |
| Upload pipeline | Track reaches `ready` status | < 60s (p95) | rolling 24h |

**Error budget math:**
```
99.9% availability → 0.1% budget
30 days × 24h × 60min = 43,200 min/month
0.1% × 43,200 = 43.2 min/month of allowed downtime
```

If error budget is burning fast → freeze non-critical deploys, focus on reliability.

### Grafana dashboards (local docker-compose)

Current: Prometheus data source pre-configured (`grafana/provisioning/`).
Access: http://localhost:3000 (admin/admin)

Dashboards to add:
- **API overview**: request rate, error rate, p99 latency by endpoint
- **Infrastructure**: CPU, memory per container, Redis memory, PG connections
- **Business**: tracks uploaded/hr, playlists created/day, active users

### CloudWatch metrics (Phase 8)

```hcl
# Alarm: high error rate
resource "aws_cloudwatch_metric_alarm" "api_5xx" {
  alarm_name          = "beatstream-api-5xx-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "HTTPCode_Target_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 60
  statistic           = "Sum"
  threshold           = 10
  alarm_actions       = [aws_sns_topic.alerts.arn]
}
```

---

## Traces (Phase 6 addition — OpenTelemetry)

### Why traces matter in a distributed system

Logs tell you *what* happened. Metrics tell you *how often*. Traces tell you *where time went*.

```
User request → nginx → api-2 → PostgreSQL (42ms) → Redis (1ms) → response
                    ↘ (async) worker → Kafka → S3

Without tracing: "this request took 95ms, one step took 42ms... which step?"
With tracing:    Waterfall view showing every span, including the async Kafka consumer
```

### Our implementation

**Instrumentation points:**
- Gin middleware → root span per HTTP request
- PostgreSQL queries → child spans via `pgx` OTel hook
- Redis calls → child spans
- Kafka produce/consume → span propagation via `traceparent` header

**Trace correlation with logs:**
```go
// In each handler, zap gets the trace_id from context
span := trace.SpanFromContext(c.Request.Context())
logger.Info("creating playlist",
    zap.String("trace_id", span.SpanContext().TraceID().String()),
    zap.String("user_id", userID),
)
```

Every log line links to a trace. Every trace links to logs.

**Local: Jaeger UI** (http://localhost:16686)
**AWS: AWS X-Ray** (compatible with OpenTelemetry SDK via ADOT collector)

### Propagation

W3C TraceContext standard:
```
Request headers:
  traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
  tracestate: (optional vendor data)
```

nginx passes these headers through to the API. API propagates to Kafka messages (header) so the worker's span is a child of the original request span.

---

## CDN, DNS & Acceleration

### CDN (CloudFront)

**What it accelerates:**
- Audio files (S3) — immutable, infinite cache TTL, 95%+ cache hit rate
- Static assets (Next.js) — served by Vercel CDN
- API responses — NOT cached at CDN (dynamic, auth-dependent)

**Cache strategy:**
```
/audio/*        → Cache-Control: max-age=31536000, immutable  (audio files never change)
/v1/*           → Cache-Control: no-store  (API, always fresh)
/_next/static/* → Cache-Control: max-age=31536000  (hashed filenames = safe forever)
```

**Origin Access Control (OAC):**
S3 bucket is private. CloudFront uses OAC (signed requests) to read from S3.
No public S3 URLs. Pre-signed URLs for direct upload only.

**Price class:**
- PriceClass_200: Asia + US + Europe (~$0.02/GB vs $0.085/GB for _ALL)
- For Taiwan users: Tokyo POP → < 5ms after cache hit

### DNS (Route53)

```
beatstream.example.com → ALIAS → CloudFront distribution
api.beatstream.example.com → ALIAS → ALB DNS
```

**Health checks:**
- Route53 health check on ALB `/healthz`
- Failover routing: primary (ap-northeast-1) → secondary (ap-southeast-1) if primary fails

**TTL strategy:**
- A/ALIAS records: 60s (fast failover during incidents)
- MX/TXT records: 3600s (email/verification, rarely changes)

### Acceleration layers

```
Browser
  ↓ DNS: Route53 → CloudFront edge
  ↓ TLS: ACM cert terminates at CloudFront
  ↓ HTTP/2: CloudFront → ALB (HTTP/1.1 to origin)
  ↓ Compression: CloudFront gzip/brotli for text content
  ↓ ALB: sticky sessions off (stateless API)
  ↓ ECS: Go API with connection pooling
  ↓ RDS Proxy: connection pooling → Aurora
```

**Why HTTP/2 matters:**
- Multiplexing: single TCP connection for multiple requests (reduces latency for browsers making 10+ simultaneous requests)
- Header compression: smaller request overhead
- CloudFront enables HTTP/2 to browsers; internal traffic uses HTTP/1.1 (nginx → ECS)

---

## Firewall layers

```
Internet
    │
    ▼
CloudFront (Layer 7 — DDoS absorption, edge caching)
    │
    ▼
AWS WAF (attached to ALB)
    ├── OWASP Core Rule Set (SQLi, XSS, path traversal)
    ├── Rate-based rule: 100 req/5min per IP on /v1/auth/*
    ├── Geo-block: block known bad country codes (optional)
    └── IP reputation list: AWS Managed Rules
    │
    ▼
ALB (Layer 4/7 — TLS termination, health checks)
    │
    ▼
Security Groups (AWS-level firewall)
    ├── ALB SG: allow 443 from 0.0.0.0/0
    ├── ECS SG: allow 8080 only from ALB SG
    ├── RDS SG: allow 5432 only from ECS SG
    └── Redis SG: allow 6379 only from ECS SG
    │
    ▼
Application layer (in the Go code)
    ├── Security headers middleware
    ├── CORS allowlist
    ├── JWT auth (RequireAuth)
    └── Login rate limit (Redis)
```

Four layers. An attacker must bypass all four.

---

## DevOps & CI/CD

### Current state

| Stage | Tool | Status |
|---|---|---|
| PR preview | Vercel GitHub App | ✅ auto-deploy on PR |
| PR comment | GitHub Actions + `deployment_status` | ✅ clean URL comment |
| Backend CI | (none yet) | ❌ |
| Production deploy | Manual `make infra-push` | ❌ manual |

### Target state (Phase 8)

```yaml
# .github/workflows/deploy.yml
on:
  push:
    branches: [main]

jobs:
  test:
    - go test ./...
    - npm run build (web)

  build-push:
    - docker build → ECR (api + worker)
    - Uses OIDC (no long-lived AWS keys)

  deploy:
    - aws ecs update-service --force-new-deployment
    - Wait for deployment to complete (health check)
    - Rollback if health check fails within 5 min
```

**GitHub Actions OIDC (no long-lived keys):**
```hcl
# Terraform: trust GitHub Actions to assume this role
resource "aws_iam_role" "github_actions" {
  assume_role_policy = jsonencode({
    Statement = [{
      Effect = "Allow"
      Principal = { Federated = aws_iam_openid_connect_provider.github.arn }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringLike = {
          "token.actions.githubusercontent.com:sub" =
            "repo:vic4code/system-design-lab:ref:refs/heads/main"
        }
      }
    }]
  })
}
```

No `AWS_ACCESS_KEY_ID` stored in GitHub secrets.
The role expires when the job ends. Least privilege: push to ECR + update ECS only.

### Deployment strategies

| Strategy | Downtime | Rollback | Cost | Use case |
|---|---|---|---|---|
| Rolling update | 0 (with health checks) | Re-deploy old image | 0 | Default for ECS |
| Blue/green | 0 | Instant (swap target group) | 2× resources during deploy | High-risk deploys |
| Canary | 0 | Remove canary weight | Small overhead | New features |
| In-place | Yes | Redeploy | Cheapest | Never for production |

**ECS rolling update (our Phase 8 default):**
- New task starts, passes health check
- ALB adds new task to target group
- ALB drains old task (default: 300s)
- Old task stops

**ECS circuit breaker:**
```hcl
deployment_circuit_breaker = {
  enable   = true
  rollback = true  # Auto-rollback if new tasks fail health check
}
```

If the new version crashes on startup → ECS automatically rolls back to previous task definition. Zero human intervention.

---

## SLA for an interview: how to answer

> *"What's your system's availability target?"*

> "We target 99.9% availability, which gives us a 43-minute monthly error budget. We measure this as (total requests - 5xx responses) / total requests over a rolling 30-day window. The budget is tracked in a CloudWatch dashboard — if we burn more than 50% of the budget in a week, deploys of non-critical features are paused until we investigate.
>
> The 99.9% target (vs 99.99%) is a deliberate tradeoff: the infrastructure cost to go from three-nines to four-nines is roughly 3–4× (multi-region active-active, cross-region RDS replica with sub-second failover). For Beatstream's current scale, the business value doesn't justify that cost."

> *"How do you know when something is wrong?"*

> "Three signals, each catching different problems: (1) CloudWatch alarm on ALB 5xx rate > 1% for 2 consecutive minutes → SNS → PagerDuty; (2) p99 latency alarm > 2s → possible DB slow query; (3) ECS circuit breaker + CloudWatch deployment insights — if a new deploy causes health check failures, it rolls back automatically and we get an alarm before users notice."
