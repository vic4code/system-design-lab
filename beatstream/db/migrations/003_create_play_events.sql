-- Play events: partitioned by month for efficient range scans and pruning.
-- Each month's data lives in its own child table, making it cheap to
-- DROP old partitions instead of running expensive DELETE statements.
CREATE TABLE IF NOT EXISTS play_events (
    id         UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id    UUID,
    track_id   UUID NOT NULL REFERENCES tracks(id),
    played_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ms_played  INT,
    source     TEXT          -- 'playlist', 'search', 'radio'
) PARTITION BY RANGE (played_at);

-- Current month partition (add more as needed, or automate with pg_partman)
CREATE TABLE IF NOT EXISTS play_events_2026_05 PARTITION OF play_events
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE IF NOT EXISTS play_events_2026_06 PARTITION OF play_events
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE INDEX IF NOT EXISTS play_events_track_idx ON play_events(track_id, played_at DESC);
CREATE INDEX IF NOT EXISTS play_events_user_idx  ON play_events(user_id, played_at DESC);
