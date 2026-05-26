"use client";

import { usePlayer } from "@/context/PlayerContext";
import type { Track } from "@/lib/api";

function fmt(ms: number) {
  const s = Math.floor(ms / 1000);
  return `${Math.floor(s / 60)}:${(s % 60).toString().padStart(2, "0")}`;
}

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
      className={`grid grid-cols-[16px_1fr_auto] gap-x-4 px-4 py-2 rounded group hover:bg-white/10 transition-colors items-center ${
        isActive ? "bg-white/10" : ""
      }`}
    >
      {/* # / play button */}
      <div className="flex items-center justify-center w-4 h-4">
        {isReady ? (
          <>
            <span
              className={`text-sm tabular-nums group-hover:hidden ${
                isActive && isPlaying ? "hidden" : isActive ? "text-accent" : "text-muted"
              }`}
            >
              {isActive && isPlaying ? "" : (index ?? "•")}
            </span>
            {isActive && isPlaying && (
              <span className="group-hover:hidden">
                <svg viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3 text-accent">
                  <path d="M2 2h3v12H2zm9 0h3v12h-3z" />
                </svg>
              </span>
            )}
            <button
              onClick={handlePlay}
              className="hidden group-hover:flex items-center justify-center text-white"
            >
              {isActive && isPlaying ? (
                <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
                  <path d="M2 2h3v12H2zm9 0h3v12h-3z" />
                </svg>
              ) : (
                <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4 ml-0.5">
                  <path d="M3 1.713a.7.7 0 0 1 1.05-.607l10.89 6.288a.7.7 0 0 1 0 1.212L4.05 14.894A.7.7 0 0 1 3 14.288V1.713z" />
                </svg>
              )}
            </button>
          </>
        ) : (
          <span className="text-xs text-subdued">{index ?? "—"}</span>
        )}
      </div>

      {/* Title + status */}
      <div className="flex items-center gap-3 min-w-0">
        <div className="w-10 h-10 shrink-0 bg-surface-2 rounded flex items-center justify-center text-subdued">
          <svg viewBox="0 0 24 24" fill="currentColor" className="w-4 h-4">
            <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z" />
          </svg>
        </div>
        <div className="min-w-0">
          <p className={`text-sm font-medium truncate ${isActive ? "text-accent" : "text-white"}`}>
            {track.title}
          </p>
          {track.status !== "ready" && (
            <span className={`text-xs ${
              track.status === "error" ? "text-red-400" :
              track.status === "processing" ? "text-blue-400" : "text-yellow-400"
            }`}>
              {track.status}
            </span>
          )}
        </div>
      </div>

      {/* Duration + remove */}
      <div className="flex items-center gap-3">
        <span className="text-sm text-muted tabular-nums">
          {track.duration_ms ? fmt(track.duration_ms) : "—"}
        </span>
        {onRemove && (
          <button
            onClick={onRemove}
            className="text-muted hover:text-white opacity-0 group-hover:opacity-100 transition-opacity text-lg leading-none"
            title="Remove"
          >
            ×
          </button>
        )}
      </div>
    </div>
  );
}
