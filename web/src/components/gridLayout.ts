import { useLayoutEffect, useRef, useState } from "react";
import type { RefObject } from "react";

// Shared layout constants for the virtualized views. These mirror design tokens
// (variables.css) but must be plain numbers for the virtualizer size math.
export const GAP = 16; // --sp-4
export const PAD_X = 24; // --sp-6 (horizontal padding, applied in render, not on the scroll box)
export const PAD_Y = 20; // --sp-5 (vertical padding)
export const META_H = 62; // AssetCard meta block: name + rating/color rows + padding + border
export const GRID_COL_MIN = 180; // matches --grid-min
export const WATERFALL_COL_TARGET = 220;

// Observes an element's content width via ResizeObserver. The same ref doubles
// as the virtualizer's scroll element, so measurement and scrolling stay in sync.
export function useContainerWidth<T extends HTMLElement>(): [RefObject<T | null>, number] {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.clientWidth);
    const ro = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) setWidth(entry.contentRect.width);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, width];
}

export interface ColumnMetrics {
  columns: number;
  columnWidth: number;
  usableWidth: number;
}

// Splits the usable width into as many `target`-wide columns as fit, then divides
// the remainder evenly so columns fill the row edge-to-edge.
export function columnMetrics(width: number, target: number): ColumnMetrics {
  const usableWidth = Math.max(0, width - PAD_X * 2);
  const columns = Math.max(1, Math.floor((usableWidth + GAP) / (target + GAP)));
  const columnWidth = (usableWidth - GAP * (columns - 1)) / columns;
  return { columns, columnWidth, usableWidth };
}

export function chunk<T>(items: T[], size: number): T[][] {
  if (size <= 0) return items.length ? [items] : [];
  const rows: T[][] = [];
  for (let i = 0; i < items.length; i += size) rows.push(items.slice(i, i + size));
  return rows;
}

export interface WaterfallItem {
  height: number;
  aspectRatio: number;
}

// Deterministic masonry sizing: derive a clamped image height from the asset's
// aspect ratio, then back-compute the exact aspectRatio the card should render so
// the estimated size equals the rendered height (avoids layout shift / jitter).
export function waterfallItem(w: number, h: number, columnWidth: number): WaterfallItem {
  if (columnWidth <= 0) return { height: META_H, aspectRatio: 1 };
  const rawAspect = w > 0 && h > 0 ? w / h : 1;
  const imageH = Math.round(
    Math.min(Math.max(columnWidth / rawAspect, 80), columnWidth * 2.2),
  );
  return { height: imageH + META_H, aspectRatio: columnWidth / imageH };
}
