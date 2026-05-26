"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import {
  addTrackToPlaylist,
  getPlaylist,
  getPlaylistTracks,
  listTracks,
  removeTrackFromPlaylist,
} from "@/lib/api";
import type { Playlist, Track } from "@/lib/api";
import TrackRow from "@/components/TrackRow";

export default function PlaylistDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [playlist, setPlaylist] = useState<Playlist | null>(null);
  const [tracks, setTracks] = useState<Track[]>([]);
  const [allTracks, setAllTracks] = useState<Track[]>([]);
  const [addId, setAddId] = useState("");

  useEffect(() => {
    getPlaylist(id).then(setPlaylist).catch(console.error);
    getPlaylistTracks(id).then((r) => setTracks(r.items)).catch(console.error);
    listTracks().then((r) => setAllTracks(r.items)).catch(console.error);
  }, [id]);

  const handleAdd = async () => {
    if (!addId) return;
    try {
      await addTrackToPlaylist(id, addId);
      const t = allTracks.find((t) => t.id === addId);
      if (t) setTracks((prev) => [...prev, t]);
      setAddId("");
    } catch (e) {
      console.error(e);
    }
  };

  const handleRemove = async (trackId: string) => {
    await removeTrackFromPlaylist(id, trackId);
    setTracks((prev) => prev.filter((t) => t.id !== trackId));
  };

  const available = allTracks.filter((t) => !tracks.find((pt) => pt.id === t.id));

  return (
    <div
      className="min-h-full"
      style={{ background: "linear-gradient(to bottom, #1e3264 0%, #121212 50%)" }}
    >
      {/* Header */}
      <div className="px-6 pt-14 pb-6 flex items-end gap-6">
        <div className="w-48 h-48 shrink-0 rounded shadow-xl bg-surface-2 flex items-center justify-center text-subdued">
          <svg viewBox="0 0 24 24" fill="currentColor" className="w-20 h-20">
            <path d="M15 6H3v2h12V6zm0 4H3v2h12v-2zM3 16h8v-2H3v2zM17 6v8.18c-.31-.11-.65-.18-1-.18-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3V8h3V6h-5z" />
          </svg>
        </div>
        <div>
          <p className="text-xs font-semibold uppercase text-white mb-2">Playlist</p>
          <h1 className="text-5xl font-extrabold mb-4">{playlist?.name ?? "…"}</h1>
          <p className="text-sm text-muted">{tracks.length} tracks</p>
        </div>
      </div>

      <div className="px-6 pb-8">
        {/* Track table */}
        {tracks.length > 0 && (
          <div className="grid grid-cols-[16px_1fr_auto] gap-x-4 px-4 pb-2 border-b border-white/10 mb-2 text-xs font-semibold text-muted uppercase tracking-wider">
            <span className="text-center">#</span>
            <span>Title</span>
            <span className="pr-6">Duration</span>
          </div>
        )}

        {tracks.length === 0 && (
          <p className="text-muted py-4">No tracks yet. Add some below.</p>
        )}

        <div className="mb-8">
          {tracks.map((t, i) => (
            <TrackRow
              key={t.id}
              track={t}
              index={i + 1}
              onRemove={() => handleRemove(t.id)}
            />
          ))}
        </div>

        {/* Add track */}
        {available.length > 0 && (
          <div className="flex gap-3 max-w-sm">
            <select
              value={addId}
              onChange={(e) => setAddId(e.target.value)}
              className="input"
            >
              <option value="">Add a track…</option>
              {available.map((t) => (
                <option key={t.id} value={t.id}>{t.title}</option>
              ))}
            </select>
            <button onClick={handleAdd} disabled={!addId} className="btn-primary shrink-0">
              Add
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
