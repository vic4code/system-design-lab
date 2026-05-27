# Phase 6 — Security Foundations

> Status: **in progress** — PR open on `feat/security`

Security is implemented in two layers — **local prototype** (this phase) and **Terraform** (Phase 8).
Each local decision maps directly to a cloud resource.

---

## What we built

### 1. Structured Logging (Observability first)

**Why first:** You can't debug, audit, or alert on security events if you can't read the logs.

**Files:** `internal/logger/logger.go`, `internal/middleware/zap_logger.go`

Replaced `gin.Logger()` with **zap** structured JSON logging.

Every request log line:
```json
{
  "level": "info",
  "ts": "2026-05-27T14:00:00Z",
  "service": "beatstream-api",
  "msg": "request",
  "status": 200,
  "method": "GET",
  "path": "/v1/tracks",
  "latency_ms": 4,
  "ip": "172.18.0.1",
  "request_id": "abc-123",
  "user_id": "uuid-here"
}
```

**AWS mapping:**
- `LOG_FORMAT=json` in ECS task definition → CloudWatch Logs
- CloudWatch Metric Filters on `status >= 500` → alarm → SNS → PagerDuty
- CloudWatch Logs Insights: `stats avg(latency_ms) by bin(5m)` — no APM agent needed

---

### 2. Audit Log Table

**Files:** `internal/middleware/audit.go`, `db/migrations/006_audit_logs.sql`

Every `POST / PUT / PATCH / DELETE` is recorded asynchronously after the handler:

```
audit_logs
├── user_id       UUID (nullable — captures unauthenticated logins too)
├── action        TEXT  e.g. "track.create", "auth.login", "playlist.track.add"
├── resource_type TEXT  e.g. "track", "playlist"
├── resource_id   UUID (nullable)
├── ip_address    INET
├── user_agent    TEXT
├── status_code   SMALLINT
├── request_id    TEXT
└── created_at    TIMESTAMPTZ (indexed DESC)
```

**Design decisions:**
- INSERT-only (no UPDATE/DELETE) — append-only audit trail
- Async goroutine — adds 0ms to response latency
- user_id nullable — captures failed logins and register calls
- Indexed on (user_id), (created_at DESC), (action), (ip_address)

**AWS mapping:**
- This table is the *application-level* audit trail (business actions)
- **CloudTrail** covers AWS API calls (IAM actions, S3 access, EC2 changes)
- **ALB access logs → S3** covers all HTTP-level traffic
- Together: complete audit coverage from AWS API → HTTP → application action

**GDPR note:** IP addresses are personal data (GDPR Art. 4(1)).
Auto-delete rows older than 90 days (scheduled job or pg_partman partition drop).

---

### 3. Security Headers (Defence in Depth)

**File:** `internal/middleware/security_headers.go`

Applied at **both** nginx and Go middleware layers — if one misconfigures, the other still applies.

| Header | Value | Protects against |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | MIME sniffing attacks |
| `X-Frame-Options` | `DENY` | Clickjacking |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Protocol downgrade, cookie hijacking |
| `Content-Security-Policy` | `default-src 'self'; frame-ancestors 'none'` | XSS, data injection |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Referer leakage |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | Browser feature abuse |

**CORS hardened:**
- Old: `Access-Control-Allow-Origin: *` (wildcard)
- New: origin-allowlist from `ALLOWED_ORIGINS` env var
- Dev: allow any `localhost`; production: only the exact frontend domain

**AWS mapping:**
- **WAF on ALB** (OWASP Core Rule Set) — blocks SQLi, XSS, path traversal at the edge
- **CloudFront** passes security headers through to the browser

---

### 4. Login Brute-Force Rate Limiting

**File:** `internal/middleware/ratelimit.go` → `LoginRateLimit()`

Strategy: sliding window counter in Redis.

```
Key:    ratelimit:login:<ip>
Limit:  5 attempts per 15 minutes per IP
On hit: 429 Too Many Requests + Retry-After header
On Redis failure: fail open (don't block legitimate users)
```

Example 429 response:
```json
{
  "error": "too many login attempts — try again later",
  "retry_after": 847
}
```

**AWS mapping:**
- **WAF rate-based rule** on ALB: 100 requests / 5 min per IP on `/v1/auth/*`
- WAF fires at the edge (before hitting ECS); application-level limit is the last defence
- **GuardDuty** detects credential stuffing patterns across multiple accounts

---

### 5. TLS in Transit

**Files:** `nginx/nginx.conf`, `nginx/certs/`

```bash
# One-time setup (requires: brew install mkcert)
make certs       # Generate cert + key
make certs-trust # Trust CA in system keychain (optional, needs sudo)
```

Config:
- HTTP (80) → HTTPS (443) permanent redirect (301)
- TLS 1.2+ only (disables TLS 1.0/1.1 per PCI-DSS)
- Strong cipher suite: ECDHE + AES-GCM / ChaCha20
- Session tickets disabled (forward secrecy)
- `server_tokens off` — hides nginx version from error pages

**AWS mapping:**
- **ACM** (AWS Certificate Manager): free TLS cert, auto-renews, no private key handling
- **ALB HTTPS listener** (port 443): terminates TLS, forwards plain HTTP to ECS on port 8080
- Inside VPC: ECS→RDS, ECS→Redis use TLS with `sslmode=require`
- **KMS** manages keys for RDS, S3 (SSE-KMS), ElastiCache encryption at rest

---

### 6. Secrets Pattern

**Current (dev):** environment variables in `docker-compose.yml`

**What NOT to do (and why):**
```yaml
# Bad: hardcoded secrets in source code
environment:
  JWT_SECRET: "my-super-secret-key"  # visible in git history forever
  DB_PASSWORD: "password123"          # exposed to anyone with repo access
```

**Pattern we follow (dev):** env vars from `.env` file (gitignored)

**AWS mapping — AWS Secrets Manager:**
```hcl
# Terraform
resource "aws_secretsmanager_secret" "jwt_secret" {
  name                    = "beatstream/jwt-secret"
  recovery_window_in_days = 7
  kms_key_id             = aws_kms_key.secrets.arn  # Customer-managed KMS key
}

# ECS task definition — secret injected at container start
secrets = [{
  name      = "JWT_SECRET"
  valueFrom = aws_secretsmanager_secret.jwt_secret.arn
}]
```

Benefits over env vars:
- Automatic rotation (Lambda hook)
- Audit trail (CloudTrail: who read which secret, when)
- KMS encryption at rest
- IAM: task role has `GetSecretValue` only for its own secrets (least privilege)
- Containers never see the actual value in `docker inspect`

---

## Interview answers

> *"What's the difference between audit logs and CloudTrail?"*
> CloudTrail records AWS API calls — who assumed an IAM role, who changed a security group, who accessed S3. It's infrastructure-level. Our `audit_logs` table records business actions — user X created playlist Y, user Z deleted their account. Both are needed: CloudTrail for infrastructure forensics, application audit logs for compliance and business logic.

> *"Why set security headers at both nginx AND the Go middleware?"*
> Defence in depth. If nginx is bypassed (direct ALB → ECS on port 8080 during a misconfiguration), the Go middleware still applies. If someone swaps the Go middleware, nginx still protects. In production we also add a WAF third layer at the ALB.

> *"How does TLS work between your services?"*
> Externally: ACM cert at ALB, TLS 1.2+, terminates at the load balancer. Inside the VPC: ECS→RDS uses `sslmode=require` (RDS enforces TLS). ECS→ElastiCache uses in-transit encryption. ECS→MSK uses SASL/TLS (IAM auth). The VPC private subnets ensure this traffic never touches the public internet.

> *"How do you handle key rotation?"*
> JWT: rotate `JWT_SECRET` in Secrets Manager → update ECS task definition → force new deployment → old tokens expire within 7 days automatically. DB password: Secrets Manager rotation with a Lambda function — no app restart needed if using connection pooling (PgBouncer/RDS Proxy handles credential refresh).

> *"What's your GDPR plan for audit log data?"*
> Audit log IP addresses are personal data. We auto-delete rows older than 90 days via a scheduled PostgreSQL job. In Phase 7 we add the GDPR erasure endpoint — when a user deletes their account, their `user_id` in `audit_logs` is set to NULL (ON DELETE SET NULL in the FK), preserving the security record without linking it to a real person.
