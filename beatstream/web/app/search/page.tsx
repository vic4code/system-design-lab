"use client";

import { useEffect, useRef, useState } from "react";
import { searchTracks } from "@/lib/api";
import type { Track } from "@/lib/api";
import TrackRow from "@/components/TrackRow";

export default function SearchPage() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Track[]>([]);
  const [loading, setLoading] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      return;
    }
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      setLoading(true);
      try {
        const r = await searchTracks(query);
        setResults(r.items);
      } catch {
        setResults([]);
      } finally {
        setLoading(false);
      }
    }, 300);
  }, [query]);

  return (
    <div
      className="min-h-full"
      style={{ background: "linear-gradient(to bottom, #2a1a3a 0%, #121212 40%)" }}
    >
      <div className="px-6 pt-14 pb-6">
        <h1 className="text-3xl font-bold mb-6">Search</h1>

        {/* Search input */}
        <div className="relative max-w-sm mb-8">
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-subdued pointer-events-none"
          >
            <circle cx="11" cy="11" r="8" />
            <path d="m21 21-4.35-4.35" />
          </svg>
          <input
            autoFocus
            type="text"
            placeholder="What do you want to play?"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full bg-surface-2 text-white rounded-full pl-10 pr-4 py-3 text-sm outline-none focus:ring-2 focus:ring-white placeholder-subdued"
          />
        </div>

        {!query && (
          <p className="text-muted text-sm">Start typing to search tracks.</p>
        )}
        {loading && <p className="text-muted text-sm">Searching…</p>}
        {!loading && query && results.length === 0 && (
          <p className="text-muted text-sm">No results for "{query}"</p>
        )}

        {results.length > 0 && (
          <div>
            <div className="grid grid-cols-[16px_1fr_auto] gap-x-4 px-4 pb-2 border-b border-white/10 mb-2 text-xs font-semibold text-muted uppercase tracking-wider">
              <span className="text-center">#</span>
              <span>Title</span>
              <span className="pr-6">Duration</span>
            </div>
            {results.map((t, i) => (
              <TrackRow key={t.id} track={t} index={i + 1} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
