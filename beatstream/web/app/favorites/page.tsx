"use client";

import { useEffect, useState } from "react";
import { listFavorites } from "@/lib/api";
import type { Track } from "@/lib/api";
import TrackRow from "@/components/TrackRow";
import { useAuth } from "@/context/AuthContext";

export default function FavoritesPage() {
  const [tracks, setTracks] = useState<Track[]>([]);
  const [loading, setLoading] = useState(true);
  const { state } = useAuth();

  useEffect(() => {
    if (state.status !== "authenticated") {
      setLoading(false);
      return;
    }
    listFavorites()
      .then((r) => setTracks(r.items))
      .finally(() => setLoading(false));
  }, [state.status]);

  if (state.status !== "authenticated") {
    return (
      <div className="p-8 text-center text-muted">
        <p className="text-xl font-bold text-white mb-2">Liked Songs</p>
        <p>Log in to see your favorites.</p>
      </div>
    );
  }

  return (
    <div
      className="min-h-full"
      style={{
        background: "linear-gradient(to bottom, #4c1d95 0%, #121212 30%)",
      }}
    >
      <div className="p-8">
        <div className="flex items-end gap-6 mb-8">
          <div className="w-56 h-56 bg-gradient-to-br from-indigo-700 to-purple-400 rounded flex items-center justify-center shadow-xl">
            <svg viewBox="0 0 24 24" fill="white" className="w-20 h-20">
              <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z" />
            </svg>
          </div>
          <div>
            <p className="text-sm font-bold uppercase text-white/70">Playlist</p>
            <h1 className="text-5xl font-black text-white mt-2">Liked Songs</h1>
            <p className="text-sm text-muted mt-4">{tracks.length} songs</p>
          </div>
        </div>

        {loading ? (
          <p className="text-muted">Loading...</p>
        ) : tracks.length === 0 ? (
          <p className="text-muted">No liked songs yet. Click the heart icon on any track to add it here.</p>
        ) : (
          <div className="flex flex-col">
            {tracks.map((t, i) => (
              <TrackRow key={t.id} track={t} index={i + 1} queue={tracks} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
