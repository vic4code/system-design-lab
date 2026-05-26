"use client";

import { usePlayer } from "@/context/PlayerContext";
import type { Track } from "@/lib/api";

function fmt(ms: number) {
  const s = Math.floor(ms / 1000);
  return `${Math.floor(s / 60)}:${(s % 60).toString().padStart(2, "0")}`;
}

const statusBadge: Record<string, string> = {
  pending: "bg-yellow-900 text-yellow-300",
  processing: "bg-blue-900 text-blue-300",
  ready: "",
  error: "bg-red-900 text-red-300",
};

type Props = {
  track: Track;
  index?: number;
  onRemove?: () => void;
};

export default function TrackRow({ track, index, onRemove }: Props) {
  const { play, pause, resume, track: current, isPlaying } = usePlayer();
  const isActive = current?.id === track.id;
  const isReady = track.status === "ready";

  const handlePlay = () => {
    if (!isReady) return;
    if (isActive) {
      isPlaying ? pause() : resume();
    } else {
      play(track);
    }
  };

  return (
    <div
      className={`flex items-center gap-4 px-4 py-2 rounded group hover:bg-surface-2 transition-colors ${
        isActive ? "bg-surface-2" : ""
      }`}
    >
      {/* Index / play button */}
      <div className="w-8 text-center">
        {isReady ? (
          <button
            onClick={handlePlay}
            className="w-8 h-8 flex items-center justify-center text-muted group-hover:text-white"
          >
            {isActive && isPlaying ? (
              <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
                <path d="M3 2h3v12H3zm7 0h3v12h-3z" />
              </svg>
            ) : (
              <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4 ml-0.5">
                <path d="M3 2l11 6-11 6V2z" />
              </svg>
            )}
          </button>
        ) : (
          <span className="text-xs text-muted">{index ?? "—"}</span>
        )}
      </div>

      {/* Title */}
      <div className="flex-1 min-w-0">
        <p className={`text-sm truncate ${isActive ? "text-accent" : "text-white"}`}>
          {track.title}
        </p>
      </div>

      {/* Status badge (only for non-ready) */}
      {track.status !== "ready" && (
        <span className={`text-xs px-2 py-0.5 rounded-full ${statusBadge[track.status]}`}>
          {track.status}
        </span>
      )}

      {/* Duration */}
      <span className="text-xs text-muted w-10 text-right">
        {track.duration_ms ? fmt(track.duration_ms) : "—"}
      </span>

      {/* Remove button */}
      {onRemove && (
        <button
          onClick={onRemove}
          className="text-muted hover:text-white ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
        >
          ×
        </button>
      )}
    </div>
  );
}
