package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool to PostgreSQL.
func Connect(dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate runs the SQL migration files embedded in the binary.
// For simplicity in Phase 0, migrations are applied inline here.
// In production, use a migration tool like golang-migrate.
func Migrate(pool *pgxpool.Pool) error {
	migrations := []string{
		migrationTracks,
		migrationPlaylists,
		migrationPlayEvents,
		migrationTrackStatus,
		migrationUsers,
		migrationAuditLogs,
	}
	for _, m := range migrations {
		if _, err := pool.Exec(context.Background(), m); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

const migrationTracks = `
CREATE TABLE IF NOT EXISTS artists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tracks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title         TEXT NOT NULL,
    artist_id     UUID NOT NULL REFERENCES artists(id),
    duration_ms   INT NOT NULL DEFAULT 0,
    audio_key     TEXT NOT NULL DEFAULT '',
    release_date  DATE,
    play_count    BIGINT DEFAULT 0,
    created_at    TIMESTAMPTZ DEFAULT now(),
    search_vector TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A')
    ) STORED
);

CREATE INDEX IF NOT EXISTS tracks_search_idx  ON tracks USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS tracks_artist_idx  ON tracks(artist_id);
CREATE INDEX IF NOT EXISTS tracks_release_idx ON tracks(release_date DESC NULLS LAST);
`

const migrationPlaylists = `
CREATE TABLE IF NOT EXISTS playlists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    owner_id    UUID,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    playlist_id UUID NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position    INT NOT NULL DEFAULT 0,
    added_at    TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (playlist_id, track_id)
);

CREATE INDEX IF NOT EXISTS playlist_tracks_playlist_idx ON playlist_tracks(playlist_id, position);
`

const migrationPlayEvents = `
CREATE TABLE IF NOT EXISTS play_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID,
    track_id   UUID NOT NULL REFERENCES tracks(id),
    played_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ms_played  INT,
    source     TEXT
);

CREATE INDEX IF NOT EXISTS play_events_track_idx ON play_events(track_id, played_at DESC);
CREATE INDEX IF NOT EXISTS play_events_user_idx  ON play_events(user_id, played_at DESC);
`

// migrationTrackStatus adds the upload pipeline status column.
// Default 'ready' keeps existing rows accessible without re-processing.
const migrationTrackStatus = `
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'ready';
`

const migrationUsers = `
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS users_email_idx ON users(email);
`

// migrationAuditLogs creates the audit trail table.
// INSERT-only: rows are never updated or deleted by the application.
// Retention: a scheduled job purges rows older than 90 days (GDPR compliance).
const migrationAuditLogs = `
CREATE TABLE IF NOT EXISTS audit_logs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        REFERENCES users(id) ON DELETE SET NULL,
    action        TEXT        NOT NULL,
    resource_type TEXT,
    resource_id   UUID,
    ip_address    INET        NOT NULL,
    user_agent    TEXT,
    status_code   SMALLINT    NOT NULL,
    request_id    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_logs_user_id_idx    ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS audit_logs_created_at_idx ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_action_idx     ON audit_logs(action);
CREATE INDEX IF NOT EXISTS audit_logs_ip_idx         ON audit_logs(ip_address);
`
