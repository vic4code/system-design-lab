// In local dev, all /v1/* requests go through the Next.js rewrite proxy → nginx → API.
// In production, NEXT_PUBLIC_API_URL points to the deployed API directly.
const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("bs_token");
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    ...(init?.headers as Record<string, string>),
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${BASE}${path}`, { ...init, headers });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `${res.status} ${res.statusText}`);
  }
  // 204 No Content
  if (res.status === 204) return undefined as T;
  return res.json();
}

// ─── Types ────────────────────────────────────────────────────────────────────

export type User = {
  id: string;
  email: string;
  name: string;
  created_at: string;
};

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

// ─── Auth ─────────────────────────────────────────────────────────────────────

export const register = (email: string, password: string, name: string) =>
  req<{ token: string; user: User }>(`/v1/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, name }),
  });

export const login = (email: string, password: string) =>
  req<{ token: string; user: User }>(`/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });

// ─── Tracks ───────────────────────────────────────────────────────────────────

export const listTracks = () =>
  req<{ items: Track[]; total: number }>(`/v1/tracks`);

export const getTrack = (id: string) => req<Track>(`/v1/tracks/${id}`);

export const searchTracks = (q: string) =>
  req<{ items: Track[]; total: number }>(`/v1/search?q=${encodeURIComponent(q)}`);

export const uploadTrack = (form: FormData) =>
  req<Track>(`/v1/tracks`, { method: "POST", body: form });

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

export const createPlaylist = (name: string) =>
  req<Playlist>(`/v1/playlists`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });

export const getPlaylistTracks = (id: string) =>
  req<{ items: Track[]; has_more: boolean; next_cursor: string }>(`/v1/playlists/${id}/tracks`);

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
