import { useRef } from "react";
import type { Asset } from "../types";
import { fileUrl, thumbUrl } from "../api/client";
import { KindIcon } from "./icons";
import { formatTime, useMediaController } from "./useMediaController";
import video from "./VideoPlayer.module.css";
import styles from "./AudioPlayer.module.css";

const SEEK_STEP = 5;

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

interface AudioPlayerProps {
  asset: Asset;
  toggleRef?: React.RefObject<(() => void) | null> | undefined;
}

// Custom audio player. Crucially the <audio> element carries NO `controls`
// attribute: a focused native <audio controls> swallows Space/Escape inside its
// user-agent shadow DOM, so no window/element listener ever fires (breaking the
// App's Space-to-toggle and the modal's Escape-to-close). Here focus lands on the
// wrapper div, keydown bubbles normally, and playback is driven by our own UI.
export function AudioPlayer({ asset, toggleRef }: AudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const c = useMediaController(audioRef, { toggleRef });
  const label = asset.display_name || asset.name;

  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.target instanceof HTMLButtonElement || e.target instanceof HTMLInputElement) return;
    const v = audioRef.current;
    if (!v) return;
    const actions: Record<string, () => void> = {
      " ": c.togglePlay,
      ArrowLeft: () => c.seekTo(v.currentTime - SEEK_STEP),
      ArrowRight: () => c.seekTo(v.currentTime + SEEK_STEP),
    };
    const act = actions[e.key];
    if (!act) return;
    e.preventDefault();
    // Mirror VideoPlayer: stop Space from reaching the App-level window listener
    // (which would also toggle via toggleRef) to guarantee a single trigger.
    if (e.key === " ") e.nativeEvent.stopPropagation();
    act();
  };

  const { current, duration, paused, muted, volume, dragging } = c;
  const pct = duration > 0 ? (current / duration) * 100 : 0;

  return (
    <div className={styles.player} tabIndex={0} onKeyDown={onKeyDown}>
      <audio ref={audioRef} src={fileUrl(asset.id)} preload="metadata" />

      <div className={styles.cover} onClick={c.togglePlay}>
        {asset.thumb ? (
          <img className={styles.waveform} src={thumbUrl(asset.id)} alt={label} />
        ) : (
          <KindIcon kind="audio" width={64} height={64} />
        )}
      </div>

      <div className={styles.bar}>
        <div
          ref={c.trackRef}
          className={`${video.scrub} ${dragging ? video.scrubbing : ""}`}
          onPointerDown={c.onPointerDown}
          onPointerMove={c.onPointerMove}
          onPointerUp={c.onPointerUp}
          onPointerCancel={c.endDrag}
        >
          <div className={video.track}>
            <div className={video.fill} style={{ width: `${pct}%` }} />
          </div>
          <div className={video.handle} style={{ left: `${pct}%` }} />
        </div>
        <div className={video.controls}>
          <button type="button" className={video.ctrl} onClick={c.togglePlay}
            aria-label={paused ? "播放" : "暂停"} title={paused ? "播放 (空格)" : "暂停 (空格)"}>
            {paused ? <IconPlay /> : <IconPause />}
          </button>
          <span className={video.time}>{formatTime(current)} / {formatTime(duration)}</span>
          <span className={video.spacer} />
          <button type="button" className={video.ctrl} onClick={c.toggleMute}
            aria-label={muted ? "取消静音" : "静音"} title={muted ? "取消静音" : "静音"}>
            <IconVolume off={muted || volume === 0} />
          </button>
          <input className={video.volume} type="range" min={0} max={1} step={0.05}
            value={muted ? 0 : volume} onChange={c.onVolumeInput} aria-label="音量" title="音量" />
        </div>
      </div>
    </div>
  );
}
