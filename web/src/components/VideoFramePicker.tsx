import { useEffect, useRef, useState } from "react";
import type { Asset } from "../types";
import { fileUrl } from "../api/client";
import styles from "./FramePicker.module.css";

// Match VideoPlayer's assumption of 30fps for single-frame stepping (#86).
const FRAME_STEP = 1 / 30;
// JPEG quality for the captured still — high, but not lossless, to keep the
// board thumbnail small.
const JPEG_QUALITY = 0.9;

interface VideoFramePickerProps {
  asset: Asset;
  initialMs?: number;
  onCapture: (blob: Blob, ms: number) => void;
  onClose: () => void;
}

// mm:ss.mmm — millisecond precision so the user can confirm the exact captured
// frame, unlike VideoPlayer's coarser mm:ss readout.
function fmt(sec: number): string {
  const s = Number.isFinite(sec) && sec > 0 ? sec : 0;
  const m = Math.floor(s / 60);
  const ss = Math.floor(s % 60).toString().padStart(2, "0");
  const ms = Math.floor((s % 1) * 1000).toString().padStart(3, "0");
  return `${m}:${ss}.${ms}`;
}

const IconPlay = () => (
  <svg viewBox="0 0 24 24" width={16} height={16} fill="currentColor" aria-hidden>
    <path d="M8 5v14l11-7z" />
  </svg>
);
const IconPause = () => (
  <svg viewBox="0 0 24 24" width={16} height={16} fill="currentColor" aria-hidden>
    <path d="M7 5h3.4v14H7zM13.6 5H17v14h-3.4z" />
  </svg>
);

// A modal that picks a single video frame: play/seek/step to the wanted moment,
// then "捕获此帧" draws the current frame to a canvas and hands the JPEG blob plus
// its timestamp (ms) back to the board. Same-origin `/file` bytes keep the canvas
// untainted, so toBlob succeeds without CORS.
export function VideoFramePicker({ asset, initialMs, onCapture, onClose }: VideoFramePickerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const trackRef = useRef<HTMLDivElement>(null);
  const draggingRef = useRef(false);
  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(0);
  const [paused, setPaused] = useState(true);
  const [dragging, setDragging] = useState(false);

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    const onTime = () => setCurrent(v.currentTime);
    const onMeta = () => {
      setDuration(Number.isFinite(v.duration) ? v.duration : 0);
      if (initialMs && initialMs > 0) v.currentTime = initialMs / 1000;
    };
    const onState = () => setPaused(v.paused);
    const pairs: [keyof HTMLMediaElementEventMap, () => void][] = [
      ["loadedmetadata", onMeta], ["timeupdate", onTime], ["seeked", onTime],
      ["play", onState], ["pause", onState],
    ];
    pairs.forEach(([e, h]) => v.addEventListener(e, h));
    return () => pairs.forEach(([e, h]) => v.removeEventListener(e, h));
  }, [initialMs]);

  const toggle = () => {
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
  const step = (dir: 1 | -1) => {
    const v = videoRef.current;
    if (!v) return;
    v.pause();
    seekTo(v.currentTime + dir * FRAME_STEP);
  };

  const capture = () => {
    const v = videoRef.current;
    if (!v) return;
    v.pause();
    const w = v.videoWidth;
    const h = v.videoHeight;
    if (w === 0 || h === 0) return;
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.drawImage(v, 0, 0, canvas.width, canvas.height);
    const ms = v.currentTime * 1000;
    canvas.toBlob(
      (blob) => {
        if (blob) onCapture(blob, ms);
      },
      "image/jpeg",
      JPEG_QUALITY,
    );
  };

  const pct = duration > 0 ? (current / duration) * 100 : 0;
  const label = asset.display_name || asset.name;

  return (
    <div className={styles.overlay} onMouseDown={onClose}>
      <div
        className={styles.dialog}
        role="dialog"
        aria-label="选取视频帧"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className={styles.title}>选取帧 · {label}</div>
        <div className={styles.preview}>
          <video ref={videoRef} src={fileUrl(asset.id)} preload="metadata" playsInline onClick={toggle} />
        </div>
        <div
          ref={trackRef}
          className={`${styles.scrub} ${dragging ? styles.scrubbing : ""}`}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
        >
          <div className={styles.track}>
            <div className={styles.fill} style={{ width: `${pct}%` }} />
          </div>
          <div className={styles.handle} style={{ left: `${pct}%` }} />
        </div>
        <div className={styles.controls}>
          <button type="button" className={styles.ctrl} onClick={toggle}
            aria-label={paused ? "播放" : "暂停"} title={paused ? "播放" : "暂停"}>
            {paused ? <IconPlay /> : <IconPause />}
          </button>
          <button type="button" className={styles.ctrl} onClick={() => step(-1)}
            aria-label="上一帧" title="上一帧">⟨</button>
          <button type="button" className={styles.ctrl} onClick={() => step(1)}
            aria-label="下一帧" title="下一帧">⟩</button>
          <span className={styles.time}>{fmt(current)} / {fmt(duration)}</span>
          <span className={styles.spacer} />
        </div>
        <div className={styles.actions}>
          <button type="button" className="btn btn-ghost" onClick={onClose}>取消</button>
          <button type="button" className="btn btn-primary" onClick={capture}>捕获此帧</button>
        </div>
      </div>
    </div>
  );
}
