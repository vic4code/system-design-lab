-- Adaptive bitrate: one row per encoded variant (64/128/320 kbps).
-- Populated by the upload worker after ffmpeg transcoding completes.
CREATE TABLE IF NOT EXISTS track_formats (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    track_id   UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    bitrate    INT  NOT NULL,
    codec      TEXT NOT NULL DEFAULT 'ogg',
    s3_key     TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(track_id, bitrate, codec)
);

CREATE INDEX IF NOT EXISTS track_formats_track_idx ON track_formats(track_id);
