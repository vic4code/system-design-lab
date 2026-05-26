// In local dev, all /v1/* requests go through the Next.js rewrite proxy → nginx → API.
// In production, NEXT_PUBLIC_API_URL points to the deployed API directly.
const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `${res.status} ${res.statusText}`);
  }
  return res.json();
}

// ─── Types ────────────────────────────────────────────────────────────────────

export type Track = {
  id: string;
  title: string;
  artist_id: string;
  duration_ms: number;
  status: "pending" | "processing" | "ready" | "error";
  play_count: number;
  release_date?: string;
  created_at: string;
};

export type Artist = {
  id: string;
  name: string;
  created_at: string;
};

export type Playlist = {
  id: string;
  name: string;
  description?: string;
  created_at: string;
};

// ─── Tracks ───────────────────────────────────────────────────────────────────

export const listTracks = () =>
  req<{ items: Track[]; total: number }>(`/v1/tracks`);

export const getTrack = (id: string) => req<Track>(`/v1/tracks/${id}`);

export const searchTracks = (q: string) =>
  req<{ items: Track[]; total: number }>(`/v1/search?q=${encodeURIComponent(q)}`);

export const uploadTrack = (form: FormData) =>
  req<Track>(`/v1/tracks`, { method: "POST", body: form });

// Stream URL — the API returns a 307 redirect to the pre-signed audio URL.
// Pass directly to <audio src> or <a href>.
export const streamUrl = (id: string) => `${BASE}/v1/tracks/${id}/stream`;

// ─── Artists ──────────────────────────────────────────────────────────────────

export const listArtists = () =>
  req<{ items: Artist[]; total: number }>(`/v1/artists`);

export const createArtist = (name: string) =>
  req<Artist>(`/v1/artists`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });

// ─── Playlists ────────────────────────────────────────────────────────────────

export const listPlaylists = () =>
  req<{ items: Playlist[]; total: number }>(`/v1/playlists`);

export const getPlaylist = (id: string) => req<Playlist>(`/v1/playlists/${id}`);

export const createPlaylist = (name: string, description?: string) =>
  req<Playlist>(`/v1/playlists`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, description }),
  });

export const getPlaylistTracks = (id: string) =>
  req<{ items: Track[]; total: number }>(`/v1/playlists/${id}/tracks`);

export const addTrackToPlaylist = (playlistId: string, trackId: string) =>
  req<void>(`/v1/playlists/${playlistId}/tracks`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ track_id: trackId }),
  });

export const removeTrackFromPlaylist = (playlistId: string, trackId: string) =>
  req<void>(`/v1/playlists/${playlistId}/tracks/${trackId}`, {
    method: "DELETE",
  });
