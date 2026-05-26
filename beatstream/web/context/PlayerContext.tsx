"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import type { Track } from "@/lib/api";
import { streamUrl } from "@/lib/api";

type PlayerState = {
  track: Track | null;
  isPlaying: boolean;
  progress: number; // 0–1
  currentTime: number; // seconds
  duration: number; // seconds
  play: (track: Track) => void;
  pause: () => void;
  resume: () => void;
  seek: (fraction: number) => void;
};

const PlayerContext = createContext<PlayerState>({
  track: null,
  isPlaying: false,
  progress: 0,
  currentTime: 0,
  duration: 0,
  play: () => {},
  pause: () => {},
  resume: () => {},
  seek: () => {},
});

export function PlayerProvider({ children }: { children: React.ReactNode }) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [track, setTrack] = useState<Track | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [progress, setProgress] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);

  useEffect(() => {
    const audio = new Audio();
    audio.preload = "metadata";

    audio.addEventListener("timeupdate", () => {
      setCurrentTime(audio.currentTime);
      setProgress(audio.duration ? audio.currentTime / audio.duration : 0);
    });
    audio.addEventListener("loadedmetadata", () => setDuration(audio.duration));
    audio.addEventListener("ended", () => setIsPlaying(false));
    audio.addEventListener("play", () => setIsPlaying(true));
    audio.addEventListener("pause", () => setIsPlaying(false));

    audioRef.current = audio;
    return () => audio.pause();
  }, []);

  const play = useCallback((t: Track) => {
    const audio = audioRef.current;
    if (!audio) return;
    audio.src = streamUrl(t.id);
    audio.load();
    audio.play().catch(console.error);
    setTrack(t);
    setProgress(0);
    setCurrentTime(0);
    setDuration(0);
  }, []);

  const pause = useCallback(() => audioRef.current?.pause(), []);
  const resume = useCallback(() => audioRef.current?.play(), []);

  const seek = useCallback((fraction: number) => {
    const audio = audioRef.current;
    if (!audio || !audio.duration) return;
    audio.currentTime = fraction * audio.duration;
  }, []);

  return (
    <PlayerContext.Provider
      value={{ track, isPlaying, progress, currentTime, duration, play, pause, resume, seek }}
    >
      {children}
    </PlayerContext.Provider>
  );
}

export const usePlayer = () => useContext(PlayerContext);
