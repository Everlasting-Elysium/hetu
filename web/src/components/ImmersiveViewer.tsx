import { useEffect, useState } from "react";
import type { Asset } from "../types";
import { fileUrl, thumbUrl } from "../api/client";
import { AssetMedia } from "./AssetDetail";
import { IconChevronLeft, IconChevronRight, IconClose } from "./icons";
import styles from "./ImmersiveViewer.module.css";

interface Props {
  assets: Asset[];
  startIndex: number;
  onExit: () => void;
}

const clamp = (i: number, max: number): number => Math.min(Math.max(i, 0), max);

// Distraction-free fullscreen viewer. One asset at a time; ←/→ navigate, Esc
// exits. Images load the original (Range-streamed) with a thumbnail fallback.
export function ImmersiveViewer({ assets, startIndex, onExit }: Props) {
  const [index, setIndex] = useState(() => clamp(startIndex, Math.max(0, assets.length - 1)));
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (assets.length === 0) onExit();
  }, [assets.length, onExit]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowLeft") setIndex((i) => Math.max(0, i - 1));
      else if (e.key === "ArrowRight") setIndex((i) => Math.min(assets.length - 1, i + 1));
      else if (e.key === "Escape") onExit();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [assets.length, onExit]);

  // Clamp when the dataset shrinks; reset the image error flag on navigation.
  useEffect(() => {
    setIndex((i) => clamp(i, Math.max(0, assets.length - 1)));
  }, [assets.length]);
  useEffect(() => {
    setFailed(false);
  }, [index]);

  const asset = assets[index];
  if (!asset) return null;
  const label = asset.display_name || asset.name;

  return (
    <div className={styles.overlay} data-testid="immersive-overlay" onClick={(e) => e.stopPropagation()}>
      <div className={styles.bar}>
        <span className={styles.name} title={label}>
          {label}
        </span>
        <span className={styles.counter}>
          {index + 1} / {assets.length}
        </span>
        <button type="button" className={styles.close} title="退出 (Esc)" onClick={onExit}>
          <IconClose />
        </button>
      </div>

      <button
        type="button"
        className={`${styles.nav} ${styles.prev}`}
        title="上一张 (←)"
        disabled={index === 0}
        onClick={() => setIndex((i) => Math.max(0, i - 1))}
      >
        <IconChevronLeft width={30} height={30} />
      </button>

      <div className={styles.stage}>
        {asset.kind === "image" ? (
          <img
            className={styles.image}
            src={failed || !asset.thumb ? thumbUrl(asset.id) : fileUrl(asset.path)}
            alt={label}
            onError={() => setFailed(true)}
          />
        ) : (
          <AssetMedia asset={asset} />
        )}
      </div>

      <button
        type="button"
        className={`${styles.nav} ${styles.next}`}
        title="下一张 (→)"
        disabled={index === assets.length - 1}
        onClick={() => setIndex((i) => Math.min(assets.length - 1, i + 1))}
      >
        <IconChevronRight width={30} height={30} />
      </button>
    </div>
  );
}
