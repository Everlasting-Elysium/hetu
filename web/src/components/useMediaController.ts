import { useCallback, useEffect, useRef, useState } from "react";
import type React from "react";

// m:ss timecode shared by both media players. Guards against NaN/negative from
// an unloaded element (duration is NaN until loadedmetadata).
export function formatTime(sec: number): string {
  const s = Number.isFinite(sec) && sec > 0 ? sec : 0;
  const m = Math.floor(s / 60);
  return `${m}:${Math.floor(s % 60).toString().padStart(2, "0")}`;
}

// Shared media-control logic for the custom video/audio players. Owns the media
// element's playback state (synced one-way from the element's events), a togglePlay
// exposed to the App-level Space handler via `toggleRef`, and pointer-drag seeking
// on a track element. Kept UI-agnostic: each player renders its own controls.
export interface MediaController {
  current: number;
  duration: number;
  paused: boolean;
  rate: number;
  muted: boolean;
  volume: number;
  dragging: boolean;
  togglePlay: () => void;
  seekTo: (t: number) => void;
  toggleMute: () => void;
  onVolumeInput: (e: React.ChangeEvent<HTMLInputElement>) => void;
  trackRef: React.RefObject<HTMLDivElement | null>;
  onPointerDown: (e: React.PointerEvent<HTMLDivElement>) => void;
  onPointerMove: (e: React.PointerEvent<HTMLDivElement>) => void;
  onPointerUp: (e: React.PointerEvent<HTMLDivElement>) => void;
  endDrag: () => void;
}

interface Options {
  // Populated so the App-level Space handler can toggle playback without the
  // player having DOM focus (e.g. focus on body while the modal is open).
  toggleRef?: React.RefObject<(() => void) | null> | undefined;
  // Runs inside the timeupdate handler before `current` is committed, letting a
  // caller adjust `currentTime` (e.g. the video A-B loop) with no extra listener.
  onTick?: (media: HTMLMediaElement) => void;
}

export function useMediaController(
  mediaRef: React.RefObject<HTMLMediaElement | null>,
  opts: Options = {},
): MediaController {
  const { toggleRef, onTick } = opts;
  const trackRef = useRef<HTMLDivElement>(null);
  const draggingRef = useRef(false);

  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(0);
  const [paused, setPaused] = useState(true);
  const [rate, setRate] = useState(1);
  const [muted, setMuted] = useState(false);
  const [volume, setVolume] = useState(1);
  const [dragging, setDragging] = useState(false);

  // Keep the tick hook in a ref so the sync effect stays attach-once (a changing
  // onTick identity must not re-subscribe the element listeners).
  const tickRef = useRef(onTick);
  tickRef.current = onTick;

  // Media state syncs one-way from the element. Attached once; reads live values.
  useEffect(() => {
    const v = mediaRef.current;
    if (!v) return;
    const onTime = () => {
      tickRef.current?.(v);
      setCurrent(v.currentTime);
    };
    const onMeta = () => {
      setDuration(Number.isFinite(v.duration) ? v.duration : 0);
      setRate(v.playbackRate);
      setMuted(v.muted);
      setVolume(v.volume);
    };
    const onState = () => setPaused(v.paused);
    const onRate = () => setRate(v.playbackRate);
    const onVol = () => {
      setMuted(v.muted);
      setVolume(v.volume);
    };
    const pairs: [keyof HTMLMediaElementEventMap, () => void][] = [
      ["loadedmetadata", onMeta], ["timeupdate", onTime], ["play", onState],
      ["pause", onState], ["ratechange", onRate], ["volumechange", onVol],
    ];
    pairs.forEach(([e, h]) => v.addEventListener(e, h));
    return () => pairs.forEach(([e, h]) => v.removeEventListener(e, h));
  }, [mediaRef]);

  const togglePlay = useCallback(() => {
    const v = mediaRef.current;
    if (!v) return;
    if (v.paused) void v.play();
    else v.pause();
  }, [mediaRef]);

  // Expose togglePlay to the App-level Space handler (fires even without focus).
  useEffect(() => {
    if (toggleRef) toggleRef.current = togglePlay;
    return () => {
      if (toggleRef) toggleRef.current = null;
    };
  }, [togglePlay, toggleRef]);

  const seekTo = useCallback(
    (t: number) => {
      const v = mediaRef.current;
      if (!v) return;
      const clamped = Math.min(duration || v.duration || 0, Math.max(0, t));
      v.currentTime = clamped;
      setCurrent(clamped);
    },
    [mediaRef, duration],
  );

  const seekFromX = useCallback(
    (clientX: number) => {
      const track = trackRef.current;
      const dur = mediaRef.current?.duration ?? duration;
      if (!track || !Number.isFinite(dur) || dur <= 0) return;
      const rect = track.getBoundingClientRect();
      seekTo(Math.min(1, Math.max(0, (clientX - rect.left) / rect.width)) * dur);
    },
    [mediaRef, duration, seekTo],
  );

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    trackRef.current?.setPointerCapture(e.pointerId);
    draggingRef.current = true;
    setDragging(true);
    seekFromX(e.clientX);
  };
  const onPointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (draggingRef.current) seekFromX(e.clientX);
  };
  const endDrag = () => {
    draggingRef.current = false;
    setDragging(false);
  };
  const onPointerUp = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!draggingRef.current) return;
    endDrag();
    trackRef.current?.releasePointerCapture(e.pointerId);
  };

  const toggleMute = () => {
    const v = mediaRef.current;
    if (v) v.muted = !v.muted;
  };
  const onVolumeInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    const v = mediaRef.current;
    if (!v) return;
    v.volume = Number(e.target.value);
    v.muted = v.volume === 0;
  };

  return {
    current, duration, paused, rate, muted, volume, dragging,
    togglePlay, seekTo, toggleMute, onVolumeInput,
    trackRef, onPointerDown, onPointerMove, onPointerUp, endDrag,
  };
}
