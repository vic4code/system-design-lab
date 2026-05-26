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
    <div className="max-w-3xl mx-auto px-6 py-8">
      <h1 className="text-2xl font-bold mb-1">{playlist?.name ?? "…"}</h1>
      <p className="text-sm text-muted mb-6">{tracks.length} tracks</p>

      <div className="space-y-1 mb-8">
        {tracks.length === 0 && <p className="text-muted">No tracks yet.</p>}
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
        <div className="flex gap-3">
          <select
            value={addId}
            onChange={(e) => setAddId(e.target.value)}
            className="input flex-1"
          >
            <option value="">Add a track…</option>
            {available.map((t) => (
              <option key={t.id} value={t.id}>
                {t.title}
              </option>
            ))}
          </select>
          <button onClick={handleAdd} disabled={!addId} className="btn-primary">
            Add
          </button>
        </div>
      )}
    </div>
  );
}
