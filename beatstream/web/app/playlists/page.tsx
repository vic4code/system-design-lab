"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { createPlaylist, listPlaylists } from "@/lib/api";
import type { Playlist } from "@/lib/api";

export default function PlaylistsPage() {
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);

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
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="max-w-3xl mx-auto px-6 py-8">
      <h1 className="text-2xl font-bold mb-6">Playlists</h1>

      {/* Create form */}
      <form onSubmit={handleCreate} className="flex gap-3 mb-8">
        <input
          type="text"
          placeholder="New playlist name"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          className="input flex-1"
        />
        <button type="submit" disabled={creating} className="btn-primary">
          Create
        </button>
      </form>

      {playlists.length === 0 && (
        <p className="text-muted">No playlists yet.</p>
      )}

      <div className="space-y-2">
        {playlists.map((p) => (
          <Link
            key={p.id}
            href={`/playlists/${p.id}`}
            className="flex items-center gap-4 px-4 py-3 rounded bg-surface hover:bg-surface-2 transition-colors"
          >
            <div className="w-10 h-10 rounded bg-surface-2 flex items-center justify-center text-accent font-bold text-lg">
              ♪
            </div>
            <div>
              <p className="font-medium">{p.name}</p>
              <p className="text-xs text-muted">
                {new Date(p.created_at).toLocaleDateString()}
              </p>
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}
