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
