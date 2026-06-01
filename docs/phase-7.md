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

**Prerequisites:** `make up && make migrate && make seed`

---

### 1. RBAC — user token hits admin endpoint → 403

```bash
# first get a token for a regular user
TOKEN=$(curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"normaluser@example.com","password":"pass123","name":"Normal User"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# hit the admin endpoint
curl -sk https://localhost/v1/admin/users \
  -H "Authorization: Bearer $TOKEN"
```

**Expected output:** `{"error":"forbidden"}` (403)

**Decode the token to confirm the role:**
```bash
echo $TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool | grep role
```

**Expected output:** `"role": "user"`

**What this demonstrates:** The `RequireRole("admin")` middleware reads the role from the JWT claim — no DB lookup required, fully stateless. The role is written into the JWT at register/login time and is valid for 7 days.

---

### 2. JWT contains a role claim — verify it directly from the token

```bash
# promote the user to admin (direct DB update, simulating an admin assignment)
USER_ID=$(curl -sk https://localhost/v1/auth/me \
  -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

docker exec beatstream-postgres-1 psql -U user -d beatstream \
  -c "UPDATE users SET role='admin' WHERE id='$USER_ID';"

# log in again to get a new token (the old token still has role=user)
ADMIN_TOKEN=$(curl -sk -X POST https://localhost/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"normaluser@example.com","password":"pass123"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# use the new token to hit the admin endpoint
curl -sk https://localhost/v1/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN" | python3 -m json.tool | head -20
```

**Expected output:** A user list with `count: N`.

**What this demonstrates:** After a role change, the user must log in again to get a new token (the old token's role claim does not update until it expires after 7 days). This is the trade-off of JWT being stateless.

---

### 3. GDPR soft delete — account becomes inaccessible, but the record is preserved

```bash
# create an account to delete
DEL_TOKEN=$(curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"delete-me@example.com","password":"pass123","name":"To Delete"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# delete the account
curl -sk -X DELETE https://localhost/v1/me \
  -H "Authorization: Bearer $DEL_TOKEN" -w "\nHTTP %{http_code}\n"
```

**Expected output:** `HTTP 204` (No Content — success)

```bash
# attempt to log in again → 401
curl -sk -X POST https://localhost/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"delete-me@example.com","password":"pass123"}'
```

**Expected output:** `{"error":"invalid email or password"}`

**Inspect the record in the DB (soft delete — row is still present):**
```bash
docker exec beatstream-postgres-1 psql -U user -d beatstream \
  -c "SELECT email, name, deleted_at FROM users WHERE email LIKE 'deleted-%' ORDER BY deleted_at DESC LIMIT 3;"
```

**Expected output:**
```
          email           |   name    |          deleted_at
--------------------------+-----------+----------------------------
 deleted-uuid@removed.in… | [deleted] | 2026-06-01 10:00:00.000...
```

**What this demonstrates:** The soft delete anonymises email/name, clears the password hash, and sets `deleted_at`. It is not a real DELETE — referential integrity is preserved (the audit_logs foreign key does not break). `audit_logs.user_id → NULL` via `ON DELETE SET NULL` is triggered when an admin performs a hard delete.

---

### 4. GDPR data export — observe a JSON dump of all the user's data

```bash
EXPORT_TOKEN=$(curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"export-test@example.com","password":"pass123","name":"Export Test"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))")

# create a few playlists
for name in "Chill" "Workout" "Study"; do
  curl -sk -X POST https://localhost/v1/playlists \
    -H "Authorization: Bearer $EXPORT_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$name\"}" > /dev/null
done

# export
curl -sk https://localhost/v1/me/export \
  -H "Authorization: Bearer $EXPORT_TOKEN" | python3 -m json.tool
```

**Expected output:**
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

**What this demonstrates:** GDPR Art. 20 (data portability) — users have the right to take their data with them. In production the correct approach is an async job + S3 pre-signed download link (synchronous response is unsuitable for large data sets).

---

### 5. Consent tracking — observe the terms_version being recorded

```bash
# specify the accepted terms version at registration
curl -sk -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"consent@example.com","password":"pass123","name":"Consent","terms_version":2}' \
  | python3 -m json.tool | grep terms

# the /me endpoint also returns it
```

**Expected output:** `"terms_version": 2`

**What this demonstrates:** When you update your privacy policy (v2, v3, …), you can query which users have not yet consented to the new version and require them to re-consent or restrict their access.
