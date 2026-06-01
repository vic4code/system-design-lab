# Phase 7 — RBAC + GDPR

> Status: **complete**

---

## What was built

### 1. Role-Based Access Control (RBAC)

- Added `role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin'))` column to `users` (migration 007)
- JWT claims now include `role`; middleware sets `UserRoleKey` in Gin context
- New `RequireRole(role string)` middleware — 403 Forbidden when claim doesn't match
- Admin endpoints protected with `requireAuth, requireAdmin` chain

### 2. GDPR Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `DELETE` | `/v1/me` | Soft-delete: sets `deleted_at`, anonymises email/name, clears password hash |
| `GET` | `/v1/me/export` | Returns JSON: profile + playlists owned by user |

Soft-delete approach:
- `email` → `deleted-<id>@removed.invalid`
- `name` → `[deleted]`
- `password_hash` → `''`
- `deleted_at` = `NOW()`
- Login query adds `AND deleted_at IS NULL` — deleted accounts cannot log in
- `audit_logs.user_id` → `NULL` via `ON DELETE SET NULL` (existing, from migration 006)

### 3. Admin Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/admin/users` | List all users (incl. soft-deleted), max 200 |
| `DELETE` | `/v1/admin/users/:id` | Hard delete — fully removes row |
| `GET` | `/v1/admin/audit-logs` | Last 500 audit log entries |

### 4. Consent Tracking

- `terms_version INT NOT NULL DEFAULT 1` stored at registration
- Returned in `GET /v1/auth/me` response
- `register` body accepts optional `terms_version` (defaults to 1)

---

## Migration: 007_rbac_gdpr.sql

```sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user'
    CHECK (role IN ('user', 'admin'));
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS terms_version INT NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS users_active_email_idx ON users(email) WHERE deleted_at IS NULL;
```

---

## Interview Answers

> *"How do you implement RBAC without a permissions table?"*
> For two roles (user/admin), a `role` column + claim in JWT is sufficient. A full RBAC table only pays off when you have many roles or dynamic permission assignment. Premature normalisation here adds four tables and three JOINs for a binary distinction.

> *"What's the difference between soft delete and GDPR erasure?"*
> Soft delete is operational — recover accidents, preserve referential integrity. GDPR erasure is a legal obligation — actually remove or anonymise PII. They co-exist: soft delete first, then a background job scrubs PII after a grace period, converting the soft delete into a true erasure.

> *"How do you handle the audit log paradox — logs contain PII, but GDPR says delete PII?"*
> `ON DELETE SET NULL` on `audit_logs.user_id` breaks the PII link while preserving the security event. IP address is handled by a 90-day purge job. This satisfies GDPR Art. 17 (right to erasure) and the legitimate interest for fraud/security (Recital 49).

> *"AWS mapping for GDPR?"*
> DynamoDB or S3 for async export job results; Step Functions for multi-step erasure workflow; EventBridge for scheduled 90-day audit log purge. Cognito groups map to the `role` claim via `custom:role` attribute in ID tokens.

---

## Files changed

- `db/migrations/007_rbac_gdpr.sql` — new
- `internal/middleware/jwt.go` — `UserRoleKey` constant, `RequireRole` middleware, propagate role in `OptionalAuth`
- `internal/handler/auth.go` — role in JWT + DB queries, `DeleteMe`, `ExportMe`, `Me` returns role/terms_version
- `internal/handler/admin.go` — new: `ListUsers`, `DeleteUser`, `ListAuditLogs`
- `cmd/api/main.go` — wire `admin` handler, `requireAdmin`, new routes

---

## Demo

**前置：** `make up && make migrate && make seed`

---

### 1. RBAC — user token 打 admin endpoint → 403

```bash
# 先拿一個普通 user 的 token
TOKEN=$(curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"normaluser@example.com","password":"pass123","name":"Normal User"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# 打 admin endpoint
curl -sk https://localhost/v1/admin/users \
  -H "Authorization: Bearer $TOKEN"
```

**你應該看到：** `{"error":"forbidden"}`（403）

**解碼 token 確認 role：**
```bash
echo $TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool | grep role
```

**你應該看到：** `"role": "user"`

**這說明了什麼：** `RequireRole("admin")` middleware 從 JWT claim 讀 role，不需要再查 DB，完全 stateless。role 在 register/login 時就寫進 JWT，7 天內有效。

---

### 2. JWT 含 role claim → 直接在 token 裡驗

```bash
# 把 user 升級成 admin（直接改 DB，模擬 admin 指派）
USER_ID=$(curl -sk https://localhost/v1/auth/me \
  -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

docker exec beatstream-postgres-1 psql -U user -d beatstream \
  -c "UPDATE users SET role='admin' WHERE id='$USER_ID';"

# 重新 login 拿新 token（舊的 token role 還是 user）
ADMIN_TOKEN=$(curl -sk -X POST https://localhost/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"normaluser@example.com","password":"pass123"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# 用新 token 打 admin endpoint
curl -sk https://localhost/v1/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN" | python3 -m json.tool | head -20
```

**你應該看到：** user 列表，`count: N`。

**這說明了什麼：** Role 改了之後，需要重新 login 取得新 token（舊 token 在 7 天 expire 前 role claim 不會自動更新）。這是 JWT stateless 的 trade-off。

---

### 3. GDPR 軟刪除 → 帳號消失但記錄保留

```bash
# 建一個待刪帳號
DEL_TOKEN=$(curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"delete-me@example.com","password":"pass123","name":"To Delete"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# 刪除自己的帳號
curl -sk -X DELETE https://localhost/v1/me \
  -H "Authorization: Bearer $DEL_TOKEN" -w "\nHTTP %{http_code}\n"
```

**你應該看到：** `HTTP 204`（No Content，成功）

```bash
# 再嘗試 login → 401
curl -sk -X POST https://localhost/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"delete-me@example.com","password":"pass123"}'
```

**你應該看到：** `{"error":"invalid email or password"}`

**看 DB 裡的記錄（軟刪除，記錄還在）：**
```bash
docker exec beatstream-postgres-1 psql -U user -d beatstream \
  -c "SELECT email, name, deleted_at FROM users WHERE email LIKE 'deleted-%' ORDER BY deleted_at DESC LIMIT 3;"
```

**你應該看到：**
```
          email           |   name    |          deleted_at
--------------------------+-----------+----------------------------
 deleted-uuid@removed.in… | [deleted] | 2026-06-01 10:00:00.000...
```

**這說明了什麼：** 軟刪除把 email/name 匿名化，password_hash 清空，設 `deleted_at`。不是真的 DELETE，保留 referential integrity（audit_logs 的 foreign key 不會 break）。`audit_logs.user_id → NULL` via `ON DELETE SET NULL` 會在 admin 做硬刪除時觸發。

---

### 4. GDPR 資料匯出 → 看到自己所有資料的 JSON

```bash
EXPORT_TOKEN=$(curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"export-test@example.com","password":"pass123","name":"Export Test"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# 建幾個 playlist
for name in "Chill" "Workout" "Study"; do
  curl -sk -X POST https://localhost/v1/playlists \
    -H "Authorization: Bearer $EXPORT_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$name\"}" > /dev/null
done

# 匯出
curl -sk https://localhost/v1/me/export \
  -H "Authorization: Bearer $EXPORT_TOKEN" | python3 -m json.tool
```

**你應該看到：**
```json
{
  "exported_at": "2026-06-01T10:00:00Z",
  "user": { "email": "export-test@example.com", "role": "user", "terms_version": 1 },
  "playlists": [
    { "name": "Chill" },
    { "name": "Workout" },
    { "name": "Study" }
  ],
  "tracks": null
}
```

**這說明了什麼：** GDPR Art. 20（資料可攜性），使用者有權拿走自己的資料。Production 作法是 async job + S3 pre-signed download link（資料量大時不適合同步回傳）。

---

### 5. Consent tracking → 看到 terms_version 記錄

```bash
# register 時指定接受的條款版本
curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"consent@example.com","password":"pass123","name":"Consent","terms_version":2}' \
  | python3 -m json.tool | grep terms

# me endpoint 也會回傳
```

**你應該看到：** `"terms_version": 2`

**這說明了什麼：** 當你更新隱私條款（v2、v3...），可以查哪些 user 還沒同意新版，要求他們重新同意或限制服務。
