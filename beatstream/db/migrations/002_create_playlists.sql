CREATE TABLE IF NOT EXISTS playlists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    owner_id    UUID,
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- Junction table linking playlists to tracks (ordered by position)
CREATE TABLE IF NOT EXISTS playlist_tracks (
    playlist_id UUID NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position    INT NOT NULL DEFAULT 0,
    added_at    TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (playlist_id, track_id)
);

-- Efficient ordered fetch for a playlist's tracks
CREATE INDEX IF NOT EXISTS playlist_tracks_playlist_idx
    ON playlist_tracks(playlist_id, position, added_at);
