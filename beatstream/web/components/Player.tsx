"use client";

import { useState } from "react";
import { usePlayer } from "@/context/PlayerContext";

function fmt(sec: number) {
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  return `${m}:${s.toString().padStart(2, "0")}`;
}

function PrevIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
      <path d="M3.3 1a.7.7 0 0 1 .7.7v5.15l9.95-5.744a.7.7 0 0 1 1.05.606v12.575a.7.7 0 0 1-1.05.607L4 9.149V14.3a.7.7 0 0 1-.7.7H1.7a.7.7 0 0 1-.7-.7V1.7a.7.7 0 0 1 .7-.7h1.6z" />
    </svg>
  );
}

function NextIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
      <path d="M12.7 1a.7.7 0 0 0-.7.7v5.15L2.05 1.107A.7.7 0 0 0 1 1.712v12.575a.7.7 0 0 0 1.05.607L12 9.149V14.3a.7.7 0 0 0 .7.7h1.6a.7.7 0 0 0 .7-.7V1.7a.7.7 0 0 0-.7-.7h-1.6z" />
    </svg>
  );
}

function VolumeIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
      <path d="M9.741.85a.75.75 0 0 1 .375.65v13a.75.75 0 0 1-1.125.65l-6.925-4a3.642 3.642 0 0 1-1.33-4.967 3.639 3.639 0 0 1 1.33-1.332l6.925-4a.75.75 0 0 1 .75 0zm-6.924 5.3a2.139 2.139 0 0 0 0 3.7l5.8 3.35V2.8l-5.8 3.35zm8.683 4.29V5.56a2.75 2.75 0 0 1 0 4.88z" />
    </svg>
  );
}

function VolumeMuteIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
      <path d="M13.86 5.47a.75.75 0 0 0-1.061 0l-1.47 1.47-1.47-1.47A.75.75 0 0 0 8.8 6.53L10.269 8l-1.47 1.47a.75.75 0 1 0 1.06 1.06l1.47-1.47 1.47 1.47a.75.75 0 0 0 1.06-1.06L12.39 8l1.47-1.47a.75.75 0 0 0 0-1.06z" />
      <path d="M10.116 1.5A.75.75 0 0 0 8.991.85l-6.925 4a3.642 3.642 0 0 0-1.33 4.967 3.639 3.639 0 0 0 1.33 1.332l6.925 4a.75.75 0 0 0 1.125-.649v-13a.75.75 0 0 0-.375-.65zm-1.5 1.3v10.4l-5.8-3.35a2.139 2.139 0 0 1 0-3.7l5.8-3.35z" />
    </svg>
  );
}

function VolumeControl({ volume, setVolume }: { volume: number; setVolume: (v: number) => void }) {
  const [prevVolume, setPrevVolume] = useState(0.7);
  const isMuted = volume === 0;

  const toggleMute = () => {
    if (isMuted) {
      setVolume(prevVolume || 0.7);
    } else {
      setPrevVolume(volume);
      setVolume(0);
    }
  };

  return (
    <div className="w-56 flex items-center justify-end gap-2">
      <button onClick={toggleMute} className="text-muted hover:text-white transition-colors">
        {isMuted ? <VolumeMuteIcon /> : <VolumeIcon />}
      </button>
      <div
        className="w-24 h-1 bg-surface-3 rounded-full relative group cursor-pointer"
        onClick={(e) => {
          const rect = e.currentTarget.getBoundingClientRect();
          const v = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
          setVolume(v);
        }}
      >
        <div
          className="h-full bg-muted group-hover:bg-accent rounded-full transition-colors"
          style={{ width: `${volume * 100}%` }}
        />
        <div
          className="absolute top-1/2 -translate-y-1/2 w-3 h-3 bg-white rounded-full opacity-0 group-hover:opacity-100 transition-opacity"
          style={{ left: `calc(${volume * 100}% - 6px)` }}
        />
      </div>
    </div>
  );
}

export default function Player() {
  const { track, isPlaying, progress, currentTime, duration, pause, resume, seek, next, prev, volume, setVolume } =
    usePlayer();

  return (
    <div className="h-[90px] bg-surface border-t border-surface-2 px-4 flex items-center justify-between gap-4 z-50 shrink-0">
      {/* Left: track info */}
      <div className="w-56 flex items-center gap-3 min-w-0">
        {track ? (
          <>
            <div className="w-14 h-14 shrink-0 bg-surface-2 rounded flex items-center justify-center text-subdued">
              <svg viewBox="0 0 24 24" fill="currentColor" className="w-6 h-6">
                <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z" />
              </svg>
            </div>
            <div className="min-w-0">
              <p className="text-sm font-medium truncate text-white">{track.title}</p>
              <p className="text-xs text-muted truncate">{track.artist_id}</p>
            </div>
          </>
        ) : (
          <div className="w-14 h-14 shrink-0 bg-surface-2 rounded" />
        )}
      </div>

      {/* Center: controls + progress */}
      <div className="flex flex-col items-center gap-2 flex-1 max-w-[45%]">
        <div className="flex items-center gap-5">
          {/* Prev */}
          <button onClick={prev} className="text-muted hover:text-white transition-colors" disabled={!track}>
            <PrevIcon />
          </button>

          {/* Play / Pause */}
          <button
            onClick={track ? (isPlaying ? pause : resume) : undefined}
            disabled={!track}
            className="w-8 h-8 rounded-full bg-white text-black flex items-center justify-center hover:scale-105 transition-transform disabled:opacity-30"
          >
            {isPlaying ? (
              <svg viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5">
                <path d="M2.7 1a.7.7 0 0 0-.7.7v12.6a.7.7 0 0 0 .7.7h2.6a.7.7 0 0 0 .7-.7V1.7a.7.7 0 0 0-.7-.7H2.7zm8 0a.7.7 0 0 0-.7.7v12.6a.7.7 0 0 0 .7.7h2.6a.7.7 0 0 0 .7-.7V1.7a.7.7 0 0 0-.7-.7h-2.6z" />
              </svg>
            ) : (
              <svg viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5 ml-0.5">
                <path d="M3 1.713a.7.7 0 0 1 1.05-.607l10.89 6.288a.7.7 0 0 1 0 1.212L4.05 14.894A.7.7 0 0 1 3 14.288V1.713z" />
              </svg>
            )}
          </button>

          {/* Next */}
          <button onClick={next} className="text-muted hover:text-white transition-colors" disabled={!track}>
            <NextIcon />
          </button>
        </div>

        {/* Progress */}
        <div className="flex items-center gap-2 w-full">
          <span className="text-xs text-subdued w-10 text-right tabular-nums">
            {track ? fmt(currentTime) : "-:--"}
          </span>
          <div
            className={`flex-1 h-1 bg-surface-3 rounded-full relative group ${track ? "cursor-pointer" : ""}`}
            onClick={track ? (e) => {
              const rect = e.currentTarget.getBoundingClientRect();
              seek((e.clientX - rect.left) / rect.width);
            } : undefined}
          >
            <div
              className="h-full bg-muted group-hover:bg-accent rounded-full transition-colors relative"
              style={{ width: `${progress * 100}%` }}
            >
              <div className="absolute right-0 top-1/2 -translate-y-1/2 w-3 h-3 bg-white rounded-full opacity-0 group-hover:opacity-100 transition-opacity" />
            </div>
          </div>
          <span className="text-xs text-subdued w-10 tabular-nums">
            {track ? fmt(duration) : "-:--"}
          </span>
        </div>
      </div>

      {/* Right: volume */}
      <VolumeControl volume={volume} setVolume={setVolume} />
    </div>
  );
}
