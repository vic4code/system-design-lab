"use client";

import { useEffect, useRef, useState } from "react";
import { createArtist, getTrack, listArtists, uploadTrack } from "@/lib/api";
import type { Artist, Track } from "@/lib/api";

type State =
  | { kind: "idle" }
  | { kind: "uploading" }
  | { kind: "polling"; track: Track }
  | { kind: "ready"; track: Track }
  | { kind: "error"; msg: string };

export default function UploadForm() {
  const [artists, setArtists] = useState<Artist[]>([]);
  const [artistId, setArtistId] = useState("");
  const [newArtistName, setNewArtistName] = useState("");
  const [title, setTitle] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [state, setState] = useState<State>({ kind: "idle" });
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    listArtists().then((r) => {
      setArtists(r.items);
      if (r.items.length > 0) setArtistId(r.items[0].id);
    });
  }, []);

  // Poll track status until ready or error
  useEffect(() => {
    if (state.kind !== "polling") {
      if (pollRef.current) clearInterval(pollRef.current);
      return;
    }
    pollRef.current = setInterval(async () => {
      try {
        const t = await getTrack(state.track.id);
        if (t.status === "ready") {
          clearInterval(pollRef.current!);
          setState({ kind: "ready", track: t });
        } else if (t.status === "error") {
          clearInterval(pollRef.current!);
          setState({ kind: "error", msg: "Transcoding failed." });
        }
      } catch {
        clearInterval(pollRef.current!);
        setState({ kind: "error", msg: "Status check failed." });
      }
    }, 2000);
    return () => clearInterval(pollRef.current!);
  }, [state]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!file || !title) return;
    setState({ kind: "uploading" });
    try {
      let aid = artistId;
      if (!aid && newArtistName) {
        const a = await createArtist(newArtistName);
        aid = a.id;
        setArtists((prev) => [...prev, a]);
        setArtistId(a.id);
      }
      if (!aid) {
        setState({ kind: "error", msg: "Select or create an artist." });
        return;
      }
      const form = new FormData();
      form.append("title", title);
      form.append("artist_id", aid);
      form.append("audio", file);
      const track = await uploadTrack(form);
      setState({ kind: "polling", track });
    } catch (err: unknown) {
      setState({ kind: "error", msg: err instanceof Error ? err.message : "Upload failed." });
    }
  };

  const reset = () => {
    setTitle("");
    setFile(null);
    setNewArtistName("");
    setState({ kind: "idle" });
  };

  if (state.kind === "ready") {
    return (
      <div className="bg-surface rounded-lg p-6 text-center space-y-3">
        <div className="text-accent text-4xl">✓</div>
        <p className="font-semibold">{state.track.title} is ready</p>
        <p className="text-sm text-muted">Track is live and can be played.</p>
        <button onClick={reset} className="btn-primary mt-2">Upload another</button>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="bg-surface rounded-lg p-6 space-y-5">
      {/* Artist */}
      <div>
        <label className="block text-sm font-medium mb-1">Artist</label>
        {artists.length > 0 ? (
          <select
            value={artistId}
            onChange={(e) => setArtistId(e.target.value)}
            className="input w-full"
          >
            {artists.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
            <option value="">+ New artist…</option>
          </select>
        ) : (
          <p className="text-sm text-muted">No artists yet — create one below.</p>
        )}
        {(artistId === "" || artists.length === 0) && (
          <input
            type="text"
            placeholder="New artist name"
            value={newArtistName}
            onChange={(e) => setNewArtistName(e.target.value)}
            className="input w-full mt-2"
          />
        )}
      </div>

      {/* Title */}
      <div>
        <label className="block text-sm font-medium mb-1">Track title</label>
        <input
          type="text"
          required
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          className="input w-full"
          placeholder="My Song"
        />
      </div>

      {/* File */}
      <div>
        <label className="block text-sm font-medium mb-1">Audio file</label>
        <input
          type="file"
          accept="audio/*"
          required
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          className="text-sm text-muted file:mr-3 file:py-1 file:px-3 file:rounded file:border-0 file:bg-surface-2 file:text-white file:cursor-pointer"
        />
      </div>

      {state.kind === "error" && (
        <p className="text-sm text-red-400">{state.msg}</p>
      )}

      {state.kind === "polling" && (
        <div className="flex items-center gap-2 text-sm text-muted">
          <span className="animate-spin">⏳</span>
          Processing… ({state.track.status})
        </div>
      )}

      <button
        type="submit"
        disabled={state.kind === "uploading" || state.kind === "polling"}
        className="btn-primary w-full disabled:opacity-50"
      >
        {state.kind === "uploading" ? "Uploading…" : "Upload"}
      </button>
    </form>
  );
}
