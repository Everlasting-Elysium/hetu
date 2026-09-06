import { useEffect, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Asset } from "../types";
import { thumbUrl } from "../api/client";
import { AssetMedia } from "./AssetDetail";
import { GridEmpty, GridError, GridSpinner } from "./GridStates";
import { RatingStars } from "./RatingStars";
import { ColorPopover } from "./ColorPicker";
import styles from "./GalleryView.module.css";

interface Props {
  assets: Asset[];
  loading: boolean;
  error: string | null;
  emptyHint: string;
  onRate: (id: string, rating: number) => void;
  onColor: (id: string, hex: string) => void;
}

const STRIP_THUMB_W = 84;
const STRIP_GAP = 8;
const STRIP_PAD = 16;

// Gallery: a large preview of the active asset above a horizontally virtualized
// filmstrip. Clicking a thumb or pressing ←/→ changes the active asset.
export function GalleryView({ assets, loading, error, emptyHint, onRate, onColor }: Props) {
  const [active, setActive] = useState(0);
  const [colorOpen, setColorOpen] = useState(false);
  const stripRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: assets.length,
    horizontal: true,
    getScrollElement: () => stripRef.current,
    estimateSize: () => STRIP_THUMB_W,
    gap: STRIP_GAP,
    overscan: 6,
  });

  // Clamp active when the dataset shrinks.
  useEffect(() => {
    if (active > assets.length - 1) setActive(Math.max(0, assets.length - 1));
  }, [assets.length, active]);

  // Keep the active thumb centered.
  useEffect(() => {
    if (assets.length) virtualizer.scrollToIndex(active, { align: "center" });
  }, [active, assets.length, virtualizer]);

  // ←/→ move the active asset (ignore while typing in a field).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = document.activeElement;
      if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return;
      if (e.key === "ArrowLeft") setActive((i) => Math.max(0, i - 1));
      else if (e.key === "ArrowRight") setActive((i) => Math.min(assets.length - 1, i + 1));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [assets.length]);

  if (loading && assets.length === 0) return <GridSpinner />;
  if (error) return <GridError message={error} />;
  if (assets.length === 0) return <GridEmpty hint={emptyHint} />;
  const current = assets[active] ?? assets[0];
  if (!current) return <GridEmpty hint={emptyHint} />;
  const label = current.display_name || current.name;

  return (
    <div className={styles.gallery} data-testid="gallery-view">
      <div className={styles.stage}>
        <AssetMedia asset={current} />
      </div>

      <div className={styles.caption}>
        <span className={styles.name} title={label}>
          {label}
        </span>
        <div className={styles.actions}>
          <RatingStars value={current.rating} onChange={(r) => onRate(current.id, r)} />
          <ColorPopover
            open={colorOpen}
            value={current.color}
            onPick={(hex) => {
              onColor(current.id, hex);
              setColorOpen(false);
            }}
          >
            <button
              type="button"
              title="颜色标签"
              className={`${styles.dot} ${current.color ? "" : styles.dotEmpty}`}
              style={current.color ? { background: current.color } : undefined}
              onClick={() => setColorOpen((o) => !o)}
            />
          </ColorPopover>
        </div>
      </div>

      <div ref={stripRef} className={styles.strip}>
        <div className={styles.stripSizer} style={{ width: virtualizer.getTotalSize() + STRIP_PAD * 2 }}>
          {virtualizer.getVirtualItems().map((item) => {
            const a = assets[item.index];
            if (!a) return null;
            return (
              <button
                key={item.key}
                type="button"
                className={`${styles.thumb} ${item.index === active ? styles.thumbActive : ""}`}
                style={{ width: item.size, transform: `translateX(${item.start + STRIP_PAD}px)` }}
                onClick={() => setActive(item.index)}
                title={a.display_name || a.name}
              >
                {a.thumb ? (
                  <img src={thumbUrl(a.id)} alt="" loading="lazy" decoding="async" />
                ) : (
                  <span className={styles.thumbFallback}>{a.ext.replace(".", "") || a.kind}</span>
                )}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
