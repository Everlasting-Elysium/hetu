import { useEffect, useRef, useState } from "react";
import type { Asset } from "../types";
import { fileUrl, thumbUrl } from "../api/client";
import styles from "./VideoPlayer.module.css";

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2] as const;
const FRAME_STEP = 1 / 30; // assume 30fps; refined via rVFC when available
const SEEK_STEP = 5;

function formatTime(sec: number): string {
  const s = Number.isFinite(sec) && sec > 0 ? sec : 0;
  const m = Math.floor(s / 60);
  return `${m}:${Math.floor(s % 60).toString().padStart(2, "0")}`;
}

// requestVideoFrameCallback ships in Chromium/WebKit but is absent from older TS
// DOM libs; feature-detect and route through a narrow unknown cast (no any).
type FrameMeta = { mediaTime: number };
function onNextFrame(v: HTMLVideoElement, cb: (meta: FrameMeta) => void): void {
  if (!("requestVideoFrameCallback" in v)) return;
  const host = v as unknown as {
    requestVideoFrameCallback: (cb: (now: number, meta: FrameMeta) => void) => number;
  };
  host.requestVideoFrameCallback((_now, meta) => cb(meta));
}

const IconPlay = () => (
  <svg viewBox="0 0 24 24" width={18} height={18} fill="currentColor" aria-hidden>
    <path d="M8 5v14l11-7z" />
  </svg>
);
const IconPause = () => (
  <svg viewBox="0 0 24 24" width={18} height={18} fill="currentColor" aria-hidden>
    <path d="M7 5h3.4v14H7zM13.6 5H17v14h-3.4z" />
  </svg>
);
const IconVolume = ({ off }: { off: boolean }) => (
  <svg viewBox="0 0 24 24" width={18} height={18} fill="none" stroke="currentColor"
    strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
    <path d="M4 9v6h4l5 4V5L8 9Z" />
    {off ? <path d="m16 9 5 6m0-6-5 6" /> : <path d="M16.5 8.5a5 5 0 0 1 0 7M19 6a8 8 0 0 1 0 12" />}
  </svg>
);

export function VideoPlayer({ asset }: { asset: Asset }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const trackRef = useRef<HTMLDivElement>(null);
  const draggingRef = useRef(false);
  const aRef = useRef<number | null>(null);
  const bRef = useRef<number | null>(null);

  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(0);
  const [paused, setPaused] = useState(true);
  const [rate, setRate] = useState(1);
  const [muted, setMuted] = useState(false);
  const [volume, setVolume] = useState(1);
  const [aPoint, setA] = useState<number | null>(null);
  const [bPoint, setB] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);

  // Media state syncs one-way from the element; the A-B loop reads refs so the
  // once-attached timeupdate handler always sees the latest markers.
  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    const onTime = () => {
      const a = aRef.current, b = bRef.current;
      if (a !== null && b !== null && a < b && v.currentTime >= b) {
        v.currentTime = a;
        setCurrent(a);
      } else setCurrent(v.currentTime);
    };
    const onMeta = () => {
      setDuration(Number.isFinite(v.duration) ? v.duration : 0);
      setRate(v.playbackRate); setMuted(v.muted); setVolume(v.volume);
    };
    const onState = () => setPaused(v.paused);
    const onRate = () => setRate(v.playbackRate);
    const onVol = () => { setMuted(v.muted); setVolume(v.volume); };
    const pairs: [keyof HTMLMediaElementEventMap, () => void][] = [
      ["loadedmetadata", onMeta], ["timeupdate", onTime], ["play", onState],
      ["pause", onState], ["ratechange", onRate], ["volumechange", onVol],
    ];
    pairs.forEach(([e, h]) => v.addEventListener(e, h));
    return () => pairs.forEach(([e, h]) => v.removeEventListener(e, h));
  }, []);

  const togglePlay = () => {
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) void v.play();
    else v.pause();
  };
  const seekTo = (t: number) => {
    const v = videoRef.current;
    if (!v) return;
    const clamped = Math.min(duration || v.duration || 0, Math.max(0, t));
    v.currentTime = clamped;
    setCurrent(clamped);
  };
  const seekFromX = (clientX: number) => {
    const track = trackRef.current;
    const dur = videoRef.current?.duration ?? duration;
    if (!track || !Number.isFinite(dur) || dur <= 0) return;
    const rect = track.getBoundingClientRect();
    seekTo(Math.min(1, Math.max(0, (clientX - rect.left) / rect.width)) * dur);
  };
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
  const stepFrame = (dir: 1 | -1) => {
    const v = videoRef.current;
    if (!v) return;
    v.pause();
    const dur = Number.isFinite(v.duration) ? v.duration : v.currentTime;
    const t = dir > 0 ? Math.min(dur, v.currentTime + FRAME_STEP) : Math.max(0, v.currentTime - FRAME_STEP);
    v.currentTime = t;
    setCurrent(t);
    if (dir > 0) onNextFrame(v, (meta) => setCurrent(meta.mediaTime));
  };
  const cycleRate = () => {
    const v = videoRef.current;
    if (!v) return;
    // Read the element (source of truth), not `rate` state: ratechange is async,
    // so two quick clicks in one render would both see the stale value and skip.
    const i = SPEEDS.findIndex((s) => s === v.playbackRate);
    v.playbackRate = SPEEDS[(i + 1) % SPEEDS.length] ?? 1;
  };
  const toggleMute = () => {
    const v = videoRef.current;
    if (v) v.muted = !v.muted;
  };
  const onVolumeInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    const v = videoRef.current;
    if (!v) return;
    v.volume = Number(e.target.value);
    v.muted = v.volume === 0;
  };

  const markA = () => {
    const v = videoRef.current;
    if (!v) return;
    aRef.current = v.currentTime;
    setA(v.currentTime);
    if (bRef.current !== null && bRef.current <= v.currentTime) { bRef.current = null; setB(null); }
  };
  const markB = () => {
    const v = videoRef.current;
    if (!v || aRef.current === null || v.currentTime <= aRef.current) return;
    bRef.current = v.currentTime;
    setB(v.currentTime);
  };
  const clearAB = () => {
    aRef.current = bRef.current = null;
    setA(null); setB(null);
  };

  // Scoped to the container (never window) so the modal keeps its own Escape.
  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.target instanceof HTMLButtonElement || e.target instanceof HTMLInputElement) return;
    const v = videoRef.current;
    if (!v) return;
    const actions: Record<string, () => void> = {
      " ": togglePlay,
      ArrowLeft: () => seekTo(v.currentTime - SEEK_STEP),
      ArrowRight: () => seekTo(v.currentTime + SEEK_STEP),
      ",": () => stepFrame(-1),
      ".": () => stepFrame(1),
    };
    const act = actions[e.key];
    if (!act) return;
    e.preventDefault();
    act();
  };
  const pct = duration > 0 ? (current / duration) * 100 : 0;
  const aPct = aPoint !== null && duration > 0 ? (aPoint / duration) * 100 : null;
  const bPct = bPoint !== null && duration > 0 ? (bPoint / duration) * 100 : null;
  const looping = aPoint !== null && bPoint !== null && aPoint < bPoint;

  return (
    <div className={styles.player} tabIndex={0} onKeyDown={onKeyDown}>
      <video
        ref={videoRef}
        className={styles.video}
        src={fileUrl(asset.id)}
        poster={thumbUrl(asset.id)}
        preload="metadata"
        playsInline
        onClick={togglePlay}
      />
      <div className={styles.bar}>
        <div
          ref={trackRef}
          className={`${styles.scrub} ${dragging ? styles.scrubbing : ""}`}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={endDrag}
        >
          <div className={styles.track}>
            <div className={styles.fill} style={{ width: `${pct}%` }} />
            {aPct !== null && <span className={styles.tick} style={{ left: `${aPct}%` }} />}
            {bPct !== null && <span className={styles.tick} style={{ left: `${bPct}%` }} />}
          </div>
          <div className={styles.handle} style={{ left: `${pct}%` }} />
        </div>
        <div className={styles.controls}>
          <button type="button" className={styles.ctrl} onClick={togglePlay}
            aria-label={paused ? "播放" : "暂停"} title={paused ? "播放 (空格)" : "暂停 (空格)"}>
            {paused ? <IconPlay /> : <IconPause />}
          </button>
          <button type="button" className={styles.ctrl} onClick={() => stepFrame(-1)}
            aria-label="上一帧" title="上一帧 (,)">⟨</button>
          <button type="button" className={styles.ctrl} onClick={() => stepFrame(1)}
            aria-label="下一帧" title="下一帧 (.)">⟩</button>
          <span className={styles.time}>{formatTime(current)} / {formatTime(duration)}</span>
          <span className={styles.spacer} />
          <button type="button" onClick={markA} aria-label="设置 A 点" title="设置 A 点"
            className={`${styles.ctrl} ${aPoint !== null ? styles.on : ""}`}>A</button>
          <button type="button" onClick={markB} aria-label="设置 B 点" title="设置 B 点"
            className={`${styles.ctrl} ${looping ? styles.on : ""}`}>B</button>
          <button type="button" onClick={clearAB} aria-label="清除 A-B 循环" title="清除 A-B 循环"
            className={styles.ctrl} disabled={aPoint === null && bPoint === null}>A-B✕</button>
          <button type="button" className={`${styles.ctrl} ${styles.rate}`} onClick={cycleRate}
            aria-label="播放速度" title="播放速度">{rate}x</button>
          <button type="button" className={styles.ctrl} onClick={toggleMute}
            aria-label={muted ? "取消静音" : "静音"} title={muted ? "取消静音" : "静音"}>
            <IconVolume off={muted || volume === 0} />
          </button>
          <input className={styles.volume} type="range" min={0} max={1} step={0.05}
            value={muted ? 0 : volume} onChange={onVolumeInput} aria-label="音量" title="音量" />
        </div>
      </div>
    </div>
  );
}
