"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  addTrackToPlaylist,
  getPlaylist,
  getPlaylistTracks,
  listTracks,
  removeTrackFromPlaylist,
  deletePlaylist,
} from "@/lib/api";
import type { Playlist, Track } from "@/lib/api";
import TrackRow from "@/components/TrackRow";
import { useAuth } from "@/context/AuthContext";

export default function PlaylistDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { state } = useAuth();
  const isAuthed = state.status === "authenticated";
  const [playlist, setPlaylist] = useState<Playlist | null>(null);
  const [tracks, setTracks] = useState<Track[]>([]);
  const [allTracks, setAllTracks] = useState<Track[]>([]);
  const [searchQuery, setSearchQuery] = useState("");
  const [showSearch, setShowSearch] = useState(false);

  useEffect(() => {
    getPlaylist(id).then(setPlaylist).catch(console.error);
    getPlaylistTracks(id).then((r) => setTracks(r.items ?? [])).catch(console.error);
    listTracks().then((r) => setAllTracks(r.items ?? [])).catch(console.error);
  }, [id]);

  const handleAdd = async (trackId: string) => {
    try {
      await addTrackToPlaylist(id, trackId);
      const t = allTracks.find((t) => t.id === trackId);
      if (t) setTracks((prev) => [...prev, t]);
    } catch (e) {
      console.error(e);
    }
  };

  const handleRemove = async (trackId: string) => {
    await removeTrackFromPlaylist(id, trackId);
    setTracks((prev) => prev.filter((t) => t.id !== trackId));
  };

  const handleDelete = async () => {
    if (!confirm("Delete this playlist?")) return;
    try {
      await deletePlaylist(id);
      router.push("/playlists");
    } catch (e) {
      console.error(e);
    }
  };

  const available = allTracks.filter(
    (t) => !tracks.find((pt) => pt.id === t.id) && t.status === "ready"
  );
  const filtered = searchQuery
    ? available.filter((t) => t.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : available.slice(0, 5);

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
          <div className="flex items-center gap-4">
            <p className="text-sm text-muted">{tracks.length} tracks</p>
            {isAuthed && (
              <button
                onClick={handleDelete}
                className="text-xs text-red-400 hover:text-red-300 transition-colors"
              >
                Delete playlist
              </button>
            )}
          </div>
        </div>
      </div>

      <div className="px-6 pb-8">
        {/* Track list */}
        {tracks.length > 0 && (
          <div className="grid grid-cols-[16px_1fr_auto] gap-x-4 px-4 pb-2 border-b border-white/10 mb-2 text-xs font-semibold text-muted uppercase tracking-wider">
            <span className="text-center">#</span>
            <span>Title</span>
            <span className="pr-6">Duration</span>
          </div>
        )}

        {tracks.length === 0 && !showSearch && (
          <p className="text-muted py-4">No tracks yet. Add some below.</p>
        )}

        <div className="mb-6">
          {tracks.map((t, i) => (
            <TrackRow
              key={t.id}
              track={t}
              index={i + 1}
              queue={tracks}
              onRemove={isAuthed ? () => handleRemove(t.id) : undefined}
            />
          ))}
        </div>

        {/* Add tracks section */}
        {isAuthed && (
          <div className="border-t border-white/10 pt-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-bold">
                {showSearch ? "Find songs to add" : ""}
              </h2>
              <button
                onClick={() => setShowSearch(!showSearch)}
                className="text-sm text-muted hover:text-white transition-colors"
              >
                {showSearch ? "Done" : "+ Add tracks"}
              </button>
            </div>

            {showSearch && (
              <div>
                <input
                  type="text"
                  placeholder="Search for songs..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="w-full max-w-md bg-surface-2 border border-white/10 rounded-md px-4 py-2 text-sm text-white placeholder:text-subdued focus:outline-none focus:border-accent mb-4"
                  autoFocus
                />
                <div className="flex flex-col gap-1">
                  {filtered.map((t) => (
                    <div
                      key={t.id}
                      className="flex items-center justify-between px-4 py-2 rounded hover:bg-white/10 transition-colors"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="w-10 h-10 shrink-0 bg-surface-2 rounded flex items-center justify-center text-subdued">
                          <svg viewBox="0 0 24 24" fill="currentColor" className="w-4 h-4">
                            <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z" />
                          </svg>
                        </div>
                        <p className="text-sm text-white truncate">{t.title}</p>
                      </div>
                      <button
                        onClick={() => handleAdd(t.id)}
                        className="text-sm border border-white/30 text-white px-3 py-1 rounded-full hover:border-white hover:scale-105 transition-all shrink-0"
                      >
                        Add
                      </button>
                    </div>
                  ))}
                  {filtered.length === 0 && (
                    <p className="text-sm text-subdued px-4 py-2">
                      {searchQuery ? "No matching tracks found." : "All tracks already added."}
                    </p>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
