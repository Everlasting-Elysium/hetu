import { useCallback, useRef, useState } from "react";
import type { Asset } from "../types";
import { fileUrl, thumbUrl } from "../api/client";
import { formatTime, useMediaController } from "./useMediaController";
import styles from "./VideoPlayer.module.css";

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2] as const;
const FRAME_STEP = 1 / 30; // assume 30fps; refined via rVFC when available
const SEEK_STEP = 5;

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

interface VideoPlayerProps {
  asset: Asset;
  // App-level Space handler populates this ref so it can call togglePlay without
  // the player having focus. When the player *does* have focus, onKeyDown handles
  // Space and calls nativeEvent.stopPropagation() to prevent the App handler from
  // seeing the same event — single-trigger guaranteed either way.
  toggleRef?: React.RefObject<(() => void) | null> | undefined;
}

export function VideoPlayer({ asset, toggleRef }: VideoPlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const aRef = useRef<number | null>(null);
  const bRef = useRef<number | null>(null);
  const [aPoint, setA] = useState<number | null>(null);
  const [bPoint, setB] = useState<number | null>(null);

  // A-B loop: on each tick, snap back to A once playback reaches B. Runs inside
  // the controller's timeupdate handler (via onTick) so `current` reflects the
  // adjusted time in the same frame — no separate listener, no jitter.
  const abLoop = useCallback((v: HTMLMediaElement) => {
    const a = aRef.current, b = bRef.current;
    if (a !== null && b !== null && a < b && v.currentTime >= b) v.currentTime = a;
  }, []);

  const c = useMediaController(videoRef, { toggleRef, onTick: abLoop });

  const stepFrame = (dir: 1 | -1) => {
    const v = videoRef.current;
    if (!v) return;
    v.pause();
    const dur = Number.isFinite(v.duration) ? v.duration : v.currentTime;
    const t = dir > 0 ? Math.min(dur, v.currentTime + FRAME_STEP) : Math.max(0, v.currentTime - FRAME_STEP);
    c.seekTo(t);
    if (dir > 0) onNextFrame(v, (meta) => c.seekTo(meta.mediaTime));
  };
  const cycleRate = () => {
    const v = videoRef.current;
    if (!v) return;
    // Read the element (source of truth), not `rate` state: ratechange is async,
    // so two quick clicks in one render would both see the stale value and skip.
    const i = SPEEDS.findIndex((s) => s === v.playbackRate);
    v.playbackRate = SPEEDS[(i + 1) % SPEEDS.length] ?? 1;
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
      " ": c.togglePlay,
      ArrowLeft: () => c.seekTo(v.currentTime - SEEK_STEP),
      ArrowRight: () => c.seekTo(v.currentTime + SEEK_STEP),
      ",": () => stepFrame(-1),
      ".": () => stepFrame(1),
    };
    const act = actions[e.key];
    if (!act) return;
    e.preventDefault();
    // Stop Space from reaching the App-level window listener to prevent double
    // trigger (App would also call togglePlay via toggleRef).
    if (e.key === " ") e.nativeEvent.stopPropagation();
    act();
  };
  const { current, duration, paused, rate, muted, volume, dragging } = c;
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
        onClick={c.togglePlay}
      />
      <div className={styles.bar}>
        <div
          ref={c.trackRef}
          className={`${styles.scrub} ${dragging ? styles.scrubbing : ""}`}
          onPointerDown={c.onPointerDown}
          onPointerMove={c.onPointerMove}
          onPointerUp={c.onPointerUp}
          onPointerCancel={c.endDrag}
        >
          <div className={styles.track}>
            <div className={styles.fill} style={{ width: `${pct}%` }} />
            {aPct !== null && <span className={styles.tick} style={{ left: `${aPct}%` }} />}
            {bPct !== null && <span className={styles.tick} style={{ left: `${bPct}%` }} />}
          </div>
          <div className={styles.handle} style={{ left: `${pct}%` }} />
        </div>
        <div className={styles.controls}>
          <button type="button" className={styles.ctrl} onClick={c.togglePlay}
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
          <button type="button" className={styles.ctrl} onClick={c.toggleMute}
            aria-label={muted ? "取消静音" : "静音"} title={muted ? "取消静音" : "静音"}>
            <IconVolume off={muted || volume === 0} />
          </button>
          <input className={styles.volume} type="range" min={0} max={1} step={0.05}
            value={muted ? 0 : volume} onChange={c.onVolumeInput} aria-label="音量" title="音量" />
        </div>
      </div>
    </div>
  );
}
