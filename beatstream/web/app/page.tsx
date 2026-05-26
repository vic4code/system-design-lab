"use client";

import { useEffect, useState } from "react";
import { listTracks } from "@/lib/api";
import type { Track } from "@/lib/api";
import TrackRow from "@/components/TrackRow";

export default function HomePage() {
  const [tracks, setTracks] = useState<Track[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    listTracks()
      .then((r) => setTracks(r.items))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return (
    <div className="max-w-3xl mx-auto px-6 py-8">
      <h1 className="text-2xl font-bold mb-6">All Tracks</h1>

      {loading && <p className="text-muted">Loading…</p>}
      {error && <p className="text-red-400">{error}</p>}

      {!loading && !error && tracks.length === 0 && (
        <p className="text-muted">No tracks yet. Upload one to get started.</p>
      )}

      <div className="space-y-1">
        {tracks.map((t, i) => (
          <TrackRow key={t.id} track={t} index={i + 1} />
        ))}
      </div>
    </div>
  );
}
