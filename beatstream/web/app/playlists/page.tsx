"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { createPlaylist, listPlaylists } from "@/lib/api";
import type { Playlist } from "@/lib/api";

const CARD_COLORS = [
  "#e91429", "#e8115b", "#8d67ab", "#477d95",
  "#1e3264", "#2d46b9", "#148a08", "#e5630a",
];

export default function PlaylistsPage() {
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [showForm, setShowForm] = useState(false);

  useEffect(() => {
    listPlaylists().then((r) => setPlaylists(r.items)).catch(console.error);
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const p = await createPlaylist(newName);
      setPlaylists((prev) => [p, ...prev]);
      setNewName("");
      setShowForm(false);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div
      className="min-h-full"
      style={{ background: "linear-gradient(to bottom, #1a2a3a 0%, #121212 40%)" }}
    >
      <div className="px-6 pt-14 pb-6">
        <div className="flex items-center justify-between mb-8">
          <h1 className="text-3xl font-bold">Your Library</h1>
          <button
            onClick={() => setShowForm((v) => !v)}
            className="w-8 h-8 rounded-full bg-surface-2 hover:bg-surface-3 flex items-center justify-center text-muted hover:text-white transition-colors text-xl leading-none"
            title="Create playlist"
          >
            +
          </button>
        </div>

        {showForm && (
          <form onSubmit={handleCreate} className="flex gap-3 mb-8 max-w-sm">
            <input
              autoFocus
              type="text"
              placeholder="Playlist name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="input"
            />
            <button type="submit" disabled={creating} className="btn-primary shrink-0">
              Create
            </button>
          </form>
        )}

        {playlists.length === 0 && (
          <p className="text-muted text-sm">No playlists yet. Hit + to create one.</p>
        )}

        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5 gap-4">
          {playlists.map((p, i) => (
            <Link
              key={p.id}
              href={`/playlists/${p.id}`}
              className="bg-surface hover:bg-surface-2 rounded p-4 transition-colors group cursor-pointer"
            >
              <div
                className="w-full aspect-square rounded mb-3 flex items-center justify-center text-white"
                style={{ background: CARD_COLORS[i % CARD_COLORS.length] }}
              >
                <svg viewBox="0 0 24 24" fill="currentColor" className="w-12 h-12 opacity-70">
                  <path d="M15 6H3v2h12V6zm0 4H3v2h12v-2zM3 16h8v-2H3v2zM17 6v8.18c-.31-.11-.65-.18-1-.18-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3V8h3V6h-5z" />
                </svg>
              </div>
              <p className="text-sm font-semibold truncate">{p.name}</p>
              <p className="text-xs text-muted mt-1">Playlist</p>
            </Link>
          ))}
        </div>
      </div>
    </div>
  );
}
