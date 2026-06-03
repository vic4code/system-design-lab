"use client";

import { useEffect, useState } from "react";
import { listTracks } from "@/lib/api";
import type { Track } from "@/lib/api";
import TrackRow from "@/components/TrackRow";

function greeting() {
  const h = new Date().getHours();
  if (h < 12) return "Good morning";
  if (h < 17) return "Good afternoon";
  return "Good evening";
}

export default function HomePage() {
  const [tracks, setTracks] = useState<Track[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    listTracks()
      .then((r) => setTracks(r.items))
      .finally(() => setLoading(false));
  }, []);

  return (
    <div
      className="min-h-full"
      style={{
        background: "linear-gradient(to bottom, #1a3a2a 0%, #121212 40%)",
      }}
    >
      <div className="px-6 pt-14 pb-6">
        <h1 className="text-3xl font-bold mb-6">{greeting()}</h1>

        {/* Track table */}
        <div>
          {/* Table header */}
          <div className="grid grid-cols-[16px_1fr_auto] gap-x-4 px-4 pb-2 border-b border-white/10 mb-2 text-xs font-semibold text-muted uppercase tracking-wider">
            <span className="text-center">#</span>
            <span>Title</span>
            <span className="pr-6">Duration</span>
          </div>

          {loading && (
            <div className="py-8 text-center text-muted text-sm">Loading…</div>
          )}

          {!loading && tracks.length === 0 && (
            <div className="py-12 text-center">
              <p className="text-muted mb-2">No tracks yet.</p>
              <p className="text-subdued text-sm">Upload one to get started.</p>
            </div>
          )}

          <div>
            {tracks.map((t, i) => (
              <TrackRow key={t.id} track={t} index={i + 1} queue={tracks} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
