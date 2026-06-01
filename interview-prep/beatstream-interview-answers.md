# Beatstream — 面試問題對照表

用自己實作的東西回答面試問題。每個問題都有「我做了什麼」+「為什麼這樣做」+「AWS 對應」三個層次。

---

## 一、系統設計類

---

### Q: 設計一個音樂串流平台（類 Spotify）

**我的實作：** Beatstream，Phase 0 → 7 完整走過一遍。

**Architecture evolution：**
- Phase 0：single Go process + Postgres + MinIO（Docker Compose）
- Phase 1：nginx 負載均衡 × 3 instances + Redis cache + rate limiter + Prometheus/Grafana
- Phase 2：Kafka（Redpanda）非同步解耦 upload pipeline 和 analytics
- Phase 3：Kubernetes，HPA 2–10 pods，StatefulSet for Postgres/Kafka
- Phase 4：Next.js frontend，Vercel 部署
- Phase 5：JWT authentication，bcrypt，multi-user
- Phase 6：structured logging（zap）、audit log table、security headers、rate limiting、TLS
- Phase 7：RBAC（role column + JWT claim）、GDPR erasure/export

**關鍵設計決策：**

1. **音訊串流用 pre-signed URL，不走 API proxy**
   - `GET /v1/tracks/:id/stream` 回傳 `307 Temporary Redirect` 到 pre-signed S3 URL（60s 有效）
   - 如果 proxy：5MB × 1000 concurrent users = 5GB/s 經過 API server，完全不可行
   - Browser 的 `<audio>` element 直接跟 S3 拿資料，API server 只處理 metadata 查詢
   - AWS：`s3.GeneratePresignedURL` 同樣機制，`S3_PRESIGN_ENDPOINT` 分開 internal vs browser-facing endpoint

2. **play_events 獨立 table，不只是 counter**
   - `play_count` 是 denormalized cache（快讀），`play_events` 是事實（可分析）
   - 有了 table 才能問：「過去 7 天播放量」「哪個小時最多人聽」
   - Phase 2 後，play event 透過 Kafka 非同步寫入，不阻塞 stream endpoint

3. **Kafka 解耦兩條 hot path**
   - Upload：audio transcoding 是 CPU-heavy，不應阻塞 HTTP response → `202 Accepted` + `status=pending` → worker 處理完 → `status=ready`
   - Analytics：高吞吐下，每次 play 直寫 Postgres = write hotspot → 先寫 Kafka，worker batch 寫入
   - Redis cache 裡絕對不 cache `status != ready` 的 track，否則 pending 會被 cache 住

---

### Q: 如何設計 caching layer？

**我的實作（Phase 1）：**
- Cache-aside pattern：先查 Redis，miss 才查 Postgres，查完寫入 Redis
- Track metadata：TTL 1 小時（變動少，容忍 stale）
- Search results：TTL 1 分鐘（變動頻繁，容忍短暫 stale）
- Cache key：`track:{id}`，`search:{query_hash}`

**Cache invalidation：**
- Track 更新時要 `DEL track:{id}`（目前是 TTL 自然過期，production 應主動 invalidate）
- **不 cache 狀態不是 ready 的 track** — 這是 Phase 2 的 gotcha：upload 剛進來時 `status=pending`，如果 cache 住，worker 處理完後 client 仍拿到 pending 狀態

**AWS mapping：** ElastiCache Redis，Multi-AZ replica，讀走 replica endpoint

---

### Q: 如何實作 rate limiting？

**我的實作（Phase 1、6）：**
- Token bucket 演算法，用 Redis `INCR` + TTL 實作
- Stream endpoint：每個 IP 每分鐘 100 次（防止音訊盜取/爬蟲）
- Login endpoint：每個 IP brute-force 保護（防密碼猜測）
- 分散式：Rate limit state 在 Redis，3 個 API instance 共用 → 一致

**為什麼 token bucket 而不是 fixed window：**
- Fixed window 的 boundary 問題：00:59 打 100 次 + 01:00 打 100 次 = 200 次在 2 秒內，繞過限制
- Token bucket 是連續的，不受時間窗口邊界影響

**AWS mapping：** API Gateway 內建 throttling；WAF Rate-based rules；或自建 Lambda@Edge + ElastiCache

---

### Q: 如何做 horizontal scaling？確保 stateless？

**我的實作（Phase 1、3）：**
- API server 完全 stateless：所有狀態在 Postgres（持久）或 Redis（ephemeral）
- nginx `least_conn` load balancing — 比 round-robin 更適合不均勻的 request duration
- Phase 3 Kubernetes HPA：CPU 60% 觸發 scale out，2 → 10 pods
- `preStop` hook + graceful shutdown：pod 被 terminate 前等待 in-flight requests 完成

**Session 問題：** JWT stateless token，不需要 sticky session。Token 有 7 天 TTL，revocation 需要 blocklist（Phase 8 課題）

**AWS mapping：** ECS Fargate + ALB Target Group；或 EKS + HPA；Spot instances 用在 worker（stateless consumer，可以 mid-process 重啟）

---

### Q: 如何做 database schema design？

**我的實作（Phase 0–7）：**

重要設計決策：

1. **UUID 主鍵，不用 auto-increment integer**
   - 分散式系統可以在 application layer 生成 ID，不依賴 DB sequence
   - `gen_random_uuid()` PostgreSQL built-in

2. **Full-text search 用 generated tsvector column**
   ```sql
   search_vector TSVECTOR GENERATED ALWAYS AS (
       setweight(to_tsvector('english', coalesce(title, '')), 'A')
   ) STORED
   ```
   - 不需要 Elasticsearch 在這個 scale；UPDATE 時自動維護，不用應用層觸發
   - GIN index 讓 `@@ plainto_tsquery` 走 index scan

3. **audit_logs append-only，ON DELETE SET NULL for user_id**
   - 安全記錄不應被刪除或修改
   - GDPR 兩難：log 含 PII，但 user 要求刪除 → `ON DELETE SET NULL` 斷開 PII link，保留 security event
   - 90 天後用 scheduled job 清除（IP 也是 PII）

4. **Partial index for active users**
   ```sql
   CREATE INDEX users_active_email_idx ON users(email) WHERE deleted_at IS NULL;
   ```
   - 99% 的 query 只查 active users，partial index 更小更快

5. **GDPR soft delete pattern**
   - `deleted_at TIMESTAMPTZ` + 匿名化 email/name
   - 不是真的 DELETE，保留 referential integrity
   - 後續 background job 做真正的 PII scrub（grace period 30 天）

---

### Q: 如何做 observability？

**我的實作（Phase 6）：**
- **Logs**：zap structured JSON，每個 request 有 `request_id`、`user_id`、`latency_ms`、`status`、`ip`
- **Metrics**：Prometheus endpoint（`:9090/metrics`），Grafana dashboard
  - `http_requests_total{method,path,status}` — counter
  - `http_request_duration_seconds{method,path}` — histogram（p50/p95/p99）
- **Traces**：OpenTelemetry，Jaeger exporter，每個 handler 有 child span for DB/Redis
- **Audit**：`audit_logs` table，每個 write operation 異步記錄

**三個 pillar 的分工：**
- Logs：What happened（事實，可查詢）
- Metrics：How it's trending（趨勢，可告警）
- Traces：Why it's slow（因果鏈，可 debug）

**AWS mapping：**
- Logs → CloudWatch Logs（`LOG_FORMAT=json` in ECS task def）
- Metrics → CloudWatch Metrics + Prometheus via ADOT
- Traces → AWS X-Ray（OTLP exporter 換成 X-Ray exporter）
- CloudTrail = AWS API level audit（不替代 application-level audit_logs）

---

## 二、Security 類

---

### Q: 如何做 API authentication？

**我的實作（Phase 5）：**
- JWT HS256，7 天 TTL
- Claims：`user_id`、`email`、`name`、`role`（Phase 7 加入）
- bcrypt hash 密碼（`DefaultCost = 10`），不儲存明文
- Middleware chain：`RequireAuth` 解 JWT → 塞 context → handler 直接取 `user_id`

**為什麼 JWT 而不是 session token：**
- Stateless：3 個 API instance 不需要共享 session store
- 缺點：token 無法即時 revoke；解法是短 TTL + refresh token（Phase 8 課題）

**Phase 7 加的 RBAC：**
- `role` column + CHECK constraint (`'user'` or `'admin'`)
- JWT claim 包含 role，middleware `RequireRole("admin")` 接在 `RequireAuth` 後
- Admin endpoints 用 `requireAuth + requireAdmin` 雙重保護
- 為什麼不做 permissions table？兩個 role 時 `role` column 夠用；permissions table 在多 role 或動態權限時才值得

**AWS mapping：** Cognito User Pools + JWT，`custom:role` attribute 注入 ID token claim

---

### Q: 如何防止常見 web 攻擊？

**我的實作（Phase 6）：**

1. **Security headers**（`internal/middleware/security_headers.go`）
   - `X-Content-Type-Options: nosniff` — 防 MIME sniffing
   - `X-Frame-Options: DENY` — 防 clickjacking
   - `Strict-Transport-Security: max-age=31536000; includeSubDomains` — HTTPS only
   - `Content-Security-Policy` — 限制 script source
   - `Referrer-Policy: strict-origin-when-cross-origin`

2. **Rate limiting**（防 brute force + DDoS）
   - Login endpoint 獨立的 brute-force limit（per IP）
   - Stream endpoint 限 100 req/min

3. **Input validation**（gin `binding` tags）
   - `binding:"required,email"` — gin 用 go-validator 在 binding 時就擋
   - SQL 全部 parameterized query，沒有字串拼接

4. **TLS**：nginx termination，API 只聽 HTTP 在 private network

**AWS mapping：** WAF（managed rule groups）、Shield（DDoS）、ACM（TLS certificates）、Security Groups（network layer）

---

### Q: GDPR compliance 怎麼做？

**我的實作（Phase 7）：**

1. **Right to erasure（被遺忘權）**：`DELETE /v1/me`
   - Soft delete：`deleted_at = NOW()`，email/name 匿名化，password_hash 清空
   - 不是硬刪除原因：保留 referential integrity，提供 30 天 grace period
   - `audit_logs.user_id` 透過 `ON DELETE SET NULL` 自動 NULL — 保留安全記錄，斷開 PII 連結

2. **Right of access（資料匯出）**：`GET /v1/me/export`
   - 同步回傳 JSON（profile + playlists）
   - 大型資料集的 production 作法：async job → S3 → pre-signed download URL

3. **Consent tracking**：`terms_version` 欄位，register 時記錄

4. **Audit log 的 PII 兩難**
   - Log 包含 IP address（是 PII）
   - 解法：`ON DELETE SET NULL` 斷 user 連結 + 90 天自動清除 IP
   - 法律依據：GDPR Art. 17（erasure）+ Recital 49（legitimate interest for security）

**AWS mapping：** Step Functions 做 async erasure workflow；EventBridge 排程 90 天 purge；DynamoDB/S3 存 export job result

---

## 三、Infrastructure / DevOps 類

---

### Q: 你如何用 IaC 管理 infrastructure？

**我的實作（Phase 3、8）：**
- Phase 3：Kubernetes YAML（`beatstream/k8s/`）— declarative，可 git track
- Phase 8：Terraform AWS infra（`beatstream/infra/`）

**Kubernetes 的重點設計：**
- Secrets vs ConfigMap 分開：credentials 進 Secret（base64 + can be encrypted at rest），non-sensitive config 進 ConfigMap
- StatefulSet for Postgres/Kafka（需要 stable network identity + persistent volume）
- HPA：CPU 60%、memory 70% 觸發 scale out，max 10 pods
- Rolling update strategy for API；Recreate for worker（Kafka consumer group 用 Recreate 避免兩個 instance 同時 rebalancing）
- `preStop` sleep 5s：讓 kube-proxy 把新連線移走，再開始 graceful shutdown

**AWS Terraform 對應（Phase 8）：**
- ECS Fargate（不管 EC2，managed）
- RDS Postgres（Multi-AZ）
- MSK（managed Kafka）
- ElastiCache Redis
- S3 + IAM role（task role，不用 access key）

---

### Q: 如何做 zero-downtime deployment？

**我的實作（Phase 3）：**
- Kubernetes Rolling Update：`maxUnavailable: 0, maxSurge: 1`
- Readiness probe：`GET /ready`（ping DB），pod 沒 ready 不送流量
- Liveness probe：`GET /healthz`，掛掉就重啟
- `preStop` hook：等 5 秒讓 LB drain 連線
- 資料庫 migration：用 `IF NOT EXISTS` 的 idempotent migration（Phase 7 的 007 migration 就是這樣寫），向前向後相容

**AWS 對應：** ECS Blue/Green deployment via CodeDeploy；ALB target group 切換；或 ECS rolling update

---

### Q: 如何監控 production 系統的健康狀況？

**我的實作（Phase 1、6）：**
- `/healthz`：always 200 OK（liveness）
- `/ready`：ping Postgres，DB 掉線時回 503（readiness）
- Prometheus metrics：`http_requests_total`、`http_request_duration_seconds`（histogram）
- Grafana dashboard：RPS、p95 latency、error rate、cache hit rate
- k6 load test：500 VUs，3m30s，325K requests，p95 = 11ms，cache hit ~99.997%

**Alerting 邏輯（AWS 版）：**
- CloudWatch Metric Filter：status 5xx count > threshold → SNS → PagerDuty
- `latency p95 > 500ms` for 5 minutes → alarm

---

## 四、Amazon LP 故事素材（Beatstream 對應）

| Leadership Principle | Beatstream 素材 |
|---|---|
| **Bias for Action** | Phase 2：Kafka idempotency 沒設好，replay 產生重複 play_count — 發現問題直接 hotfix，不等 code review cycle |
| **Dive Deep** | Go version drift bug（Phase 2）：local dev 跑得過，CI 掛 — 深挖 go.mod，發現 franz-go 的 idempotent producer config 在特定 Go 版本行為不同 |
| **Customer Obsession** | Phase 6 加 security headers、audit log — JD 沒有要求，主動加入因為 production 系統應該有 |
| **Deliver Results** | Phase 1 load test：325K requests / 3.5 分鐘，p95 11ms，用具體數字說 |
| **Think Big** | 整個 Phase 0 → 7 的架構演進：刻意從 monolith 開始，一步步拆解，每個 phase 對應一個真實的 production concern |
| **Invent and Simplify** | Phase 7 RBAC：用 role column 而不是 permissions table — 兩個 role 不需要四張表三個 JOIN，premature normalisation |

---

## 五、快問快答（口頭面試常見）

**Q: Redis 怎麼知道 cache 失效？**
TTL 自然過期；或 write-through（更新 DB 同時更新 cache）；或 explicit delete on update。Beatstream 目前用 TTL，production 應加主動 invalidation。

**Q: Kafka consumer group 是什麼？**
同一個 consumer group 裡，每個 partition 只會被一個 consumer 消費。Beatstream 的 `upload-workers` group：scale 出去多個 worker instance，Kafka 自動做 partition assignment，不會重複消費。

**Q: 為什麼 worker deployment 用 Recreate 而不是 RollingUpdate？**
Kafka consumer rebalance 發生在 consumer 加入或離開 group 時。Rolling update 期間新舊 pod 並存，group 會做兩次 rebalance，造成 lag spike 和短暫 double processing。Recreate 先 kill 舊的再啟動新的，只有一次 rebalance。

**Q: 什麼情況下用 DynamoDB 而不是 Postgres？**
單純的 key-value or key-range access，不需要 JOIN，QPS 極高（Postgres 約 10K–50K，DynamoDB 可以 millions）。如果需要複雜 query、aggregation、transaction，Postgres 更合適。

**Q: JWT 的 stateless 有什麼缺點？**
Token 無法即時 revoke（logout 了，token 還能用直到 expire）。解法：短 TTL（15 分鐘）+ refresh token；或維護 server-side blocklist（但這就不是 stateless 了）。
