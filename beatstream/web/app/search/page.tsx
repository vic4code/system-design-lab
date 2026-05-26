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
    <div className="max-w-3xl mx-auto px-6 py-8">
      <h1 className="text-2xl font-bold mb-6">Search</h1>

      <input
        autoFocus
        type="text"
        placeholder="Track title, artist…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="input w-full mb-6"
      />

      {loading && <p className="text-muted text-sm">Searching…</p>}

      {!loading && query && results.length === 0 && (
        <p className="text-muted text-sm">No results for "{query}"</p>
      )}

      <div className="space-y-1">
        {results.map((t, i) => (
          <TrackRow key={t.id} track={t} index={i + 1} />
        ))}
      </div>
    </div>
  );
}
