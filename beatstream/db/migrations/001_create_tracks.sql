-- Artists table
CREATE TABLE IF NOT EXISTS artists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- Tracks table with full-text search vector
CREATE TABLE IF NOT EXISTS tracks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title         TEXT NOT NULL,
    artist_id     UUID NOT NULL REFERENCES artists(id),
    duration_ms   INT NOT NULL DEFAULT 0,
    audio_key     TEXT NOT NULL DEFAULT '',      -- S3/MinIO object key
    release_date  DATE,
    play_count    BIGINT DEFAULT 0,
    created_at    TIMESTAMPTZ DEFAULT now(),
    -- Auto-maintained full-text search vector (title weighted A)
    search_vector TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A')
    ) STORED
);

CREATE INDEX IF NOT EXISTS tracks_search_idx  ON tracks USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS tracks_artist_idx  ON tracks(artist_id);
CREATE INDEX IF NOT EXISTS tracks_release_idx ON tracks(release_date DESC NULLS LAST);
