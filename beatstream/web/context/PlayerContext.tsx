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
  progress: number;
  currentTime: number;
  duration: number;
  volume: number;
  queue: Track[];
  play: (track: Track, queue?: Track[]) => void;
  pause: () => void;
  resume: () => void;
  seek: (fraction: number) => void;
  setVolume: (v: number) => void;
  next: () => void;
  prev: () => void;
};

const PlayerContext = createContext<PlayerState>({
  track: null,
  isPlaying: false,
  progress: 0,
  currentTime: 0,
  duration: 0,
  volume: 0.7,
  queue: [],
  play: () => {},
  pause: () => {},
  resume: () => {},
  seek: () => {},
  setVolume: () => {},
  next: () => {},
  prev: () => {},
});

export function PlayerProvider({ children }: { children: React.ReactNode }) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [track, setTrack] = useState<Track | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [progress, setProgress] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const queueRef = useRef<Track[]>([]);
  const indexRef = useRef(-1);
  const [queue, setQueue] = useState<Track[]>([]);
  const [volume, setVolumeState] = useState(0.7);

  const playByIndex = useCallback((idx: number) => {
    const audio = audioRef.current;
    const q = queueRef.current;
    if (!audio || idx < 0 || idx >= q.length) return;
    const t = q[idx];
    indexRef.current = idx;
    audio.src = streamUrl(t.id);
    audio.load();
    audio.play().catch(console.error);
    setTrack(t);
    setProgress(0);
    setCurrentTime(0);
    setDuration(0);
  }, []);

  useEffect(() => {
    const audio = new Audio();
    audio.preload = "metadata";
    audio.volume = 0.7;

    audio.addEventListener("timeupdate", () => {
      setCurrentTime(audio.currentTime);
      setProgress(audio.duration ? audio.currentTime / audio.duration : 0);
    });
    audio.addEventListener("loadedmetadata", () => setDuration(audio.duration));
    audio.addEventListener("ended", () => {
      setIsPlaying(false);
      const nextIdx = indexRef.current + 1;
      if (nextIdx < queueRef.current.length) {
        playByIndex(nextIdx);
      }
    });
    audio.addEventListener("play", () => setIsPlaying(true));
    audio.addEventListener("pause", () => setIsPlaying(false));

    audioRef.current = audio;
    return () => audio.pause();
  }, [playByIndex]);

  const play = useCallback((t: Track, newQueue?: Track[]) => {
    const audio = audioRef.current;
    if (!audio) return;

    if (newQueue) {
      queueRef.current = newQueue;
      setQueue(newQueue);
      const idx = newQueue.findIndex((q) => q.id === t.id);
      indexRef.current = idx >= 0 ? idx : 0;
    }

    audio.src = streamUrl(t.id);
    audio.load();
    audio.play().catch(console.error);
    setTrack(t);
    setProgress(0);
    setCurrentTime(0);
    setDuration(0);
  }, []);

  const pause = useCallback(() => audioRef.current?.pause(), []);
  const resume = useCallback(() => { audioRef.current?.play(); }, []);

  const seek = useCallback((fraction: number) => {
    const audio = audioRef.current;
    if (!audio || !audio.duration) return;
    audio.currentTime = fraction * audio.duration;
  }, []);

  const next = useCallback(() => {
    const nextIdx = indexRef.current + 1;
    if (nextIdx < queueRef.current.length) {
      playByIndex(nextIdx);
    }
  }, [playByIndex]);

  const prev = useCallback(() => {
    const audio = audioRef.current;
    if (audio && audio.currentTime > 3) {
      audio.currentTime = 0;
      return;
    }
    const prevIdx = indexRef.current - 1;
    if (prevIdx >= 0) {
      playByIndex(prevIdx);
    }
  }, [playByIndex]);

  const setVolume = useCallback((v: number) => {
    const audio = audioRef.current;
    if (audio) audio.volume = v;
    setVolumeState(v);
  }, []);

  return (
    <PlayerContext.Provider
      value={{ track, isPlaying, progress, currentTime, duration, volume, queue, play, pause, resume, seek, setVolume, next, prev }}
    >
      {children}
    </PlayerContext.Provider>
  );
}

export const usePlayer = () => useContext(PlayerContext);
