-- Phase 7: RBAC + GDPR

-- Role column: 'user' (default) or 'admin'
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user'
    CHECK (role IN ('user', 'admin'));

-- Soft-delete: GDPR erasure sets this; active users have NULL
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Consent tracking: version of terms accepted at registration
ALTER TABLE users ADD COLUMN IF NOT EXISTS terms_version INT NOT NULL DEFAULT 1;

-- Partial index: most queries only touch active users
CREATE INDEX IF NOT EXISTS users_active_email_idx ON users(email) WHERE deleted_at IS NULL;

-- Allow audit_log rows to survive after user erasure (break PII link, keep security record)
-- audit_logs.user_id already has ON DELETE SET NULL from migration 006
