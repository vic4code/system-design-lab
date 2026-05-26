"use client";

import { usePlayer } from "@/context/PlayerContext";

function fmt(sec: number) {
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export default function Player() {
  const { track, isPlaying, progress, currentTime, duration, pause, resume, seek } =
    usePlayer();

  if (!track) return null;

  return (
    <div className="fixed bottom-0 left-0 right-0 bg-surface border-t border-surface-2 px-4 py-3 flex items-center gap-4 z-50">
      {/* Track info */}
      <div className="w-48 truncate">
        <p className="text-sm font-medium truncate">{track.title}</p>
        <p className="text-xs text-muted truncate">{track.artist_id}</p>
      </div>

      {/* Controls */}
      <div className="flex-1 flex flex-col items-center gap-1">
        <button
          onClick={isPlaying ? pause : resume}
          className="w-8 h-8 rounded-full bg-white text-black flex items-center justify-center hover:scale-105 transition-transform"
        >
          {isPlaying ? (
            /* Pause icon */
            <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
              <path d="M3 2h3v12H3zm7 0h3v12h-3z" />
            </svg>
          ) : (
            /* Play icon */
            <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4 ml-0.5">
              <path d="M3 2l11 6-11 6V2z" />
            </svg>
          )}
        </button>

        {/* Progress bar */}
        <div className="flex items-center gap-2 w-full max-w-lg">
          <span className="text-xs text-muted w-8 text-right">{fmt(currentTime)}</span>
          <div
            className="flex-1 h-1 bg-surface-2 rounded-full cursor-pointer relative group"
            onClick={(e) => {
              const rect = e.currentTarget.getBoundingClientRect();
              seek((e.clientX - rect.left) / rect.width);
            }}
          >
            <div
              className="h-full bg-muted group-hover:bg-accent rounded-full transition-colors"
              style={{ width: `${progress * 100}%` }}
            />
          </div>
          <span className="text-xs text-muted w-8">{fmt(duration)}</span>
        </div>
      </div>

      <div className="w-48" />
    </div>
  );
}
