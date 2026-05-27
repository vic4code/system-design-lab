-- audit_logs: immutable record of every state-changing operation.
--
-- Design notes:
--   - Rows are INSERT-only (no UPDATE, no DELETE) — append-only log
--   - user_id is nullable: unauthenticated requests (register, login) are still logged
--   - resource_id is nullable: some actions (auth.register) don't have a resource yet
--   - status_code lets us query "failed deletion attempts" etc.
--   - ip_address + user_agent support forensic analysis
--
-- GDPR note: IP addresses are personal data under GDPR Art. 4(1).
-- Retention policy: auto-delete rows older than 90 days via a scheduled job
-- (see internal/worker/cleanup.go) or pg_partman if volume grows.
--
-- AWS equivalent: CloudTrail captures AWS API calls; ALB access logs capture
-- all HTTP traffic. This table is the *application-level* audit trail
-- that knows about business actions (e.g. "user X removed track Y from playlist Z").

CREATE TABLE IF NOT EXISTS audit_logs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        REFERENCES users(id) ON DELETE SET NULL,
    action        TEXT        NOT NULL,          -- e.g. "track.create", "auth.login"
    resource_type TEXT,                          -- e.g. "track", "playlist", "user"
    resource_id   UUID,                          -- ID of the affected resource
    ip_address    INET        NOT NULL,
    user_agent    TEXT,
    status_code   SMALLINT    NOT NULL,
    request_id    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Queries: "all actions by user X" and "all logins from IP Y"
CREATE INDEX IF NOT EXISTS audit_logs_user_id_idx    ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS audit_logs_created_at_idx ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_action_idx     ON audit_logs(action);
CREATE INDEX IF NOT EXISTS audit_logs_ip_idx         ON audit_logs(ip_address);
