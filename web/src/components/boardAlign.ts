import type { BoardItem } from "../types";

// The eleven arrange operations the align toolbar exposes. Grouped as: six
// edge/center alignments, two even-distributions, and three size matches.
export type AlignOp =
  | "left"
  | "centerH"
  | "right"
  | "top"
  | "middle"
  | "bottom"
  | "distributeH"
  | "distributeV"
  | "matchW"
  | "matchH"
  | "matchSize";

const avg = (ns: number[]): number => ns.reduce((s, n) => s + n, 0) / ns.length;

// Horizontal edge/center alignment: min -> left edges, max -> right edges,
// center -> every item centered on the group's average center-x.
function alignX(items: BoardItem[], mode: "min" | "center" | "max"): BoardItem[] {
  const minX = Math.min(...items.map((it) => it.x));
  const maxR = Math.max(...items.map((it) => it.x + it.w));
  const cx = avg(items.map((it) => it.x + it.w / 2));
  return items.map((it) => {
    const x = mode === "min" ? minX : mode === "max" ? maxR - it.w : cx - it.w / 2;
    return { ...it, x };
  });
}

function alignY(items: BoardItem[], mode: "min" | "center" | "max"): BoardItem[] {
  const minY = Math.min(...items.map((it) => it.y));
  const maxB = Math.max(...items.map((it) => it.y + it.h));
  const cy = avg(items.map((it) => it.y + it.h / 2));
  return items.map((it) => {
    const y = mode === "min" ? minY : mode === "max" ? maxB - it.h : cy - it.h / 2;
    return { ...it, y };
  });
}

// Even horizontal distribution: keep the leftmost/rightmost fixed and give the
// items in between equal gaps. A no-op below three items (nothing to space out).
function distributeX(items: BoardItem[]): BoardItem[] {
  if (items.length < 3) return items;
  const sorted = [...items].sort((a, b) => a.x - b.x);
  const first = sorted[0];
  const last = sorted[sorted.length - 1];
  if (!first || !last) return items;
  const span = last.x + last.w - first.x;
  const gap = (span - sorted.reduce((s, it) => s + it.w, 0)) / (sorted.length - 1);
  const pos = new Map<string, number>();
  let cursor = first.x;
  for (const it of sorted) {
    pos.set(it.id, cursor);
    cursor += it.w + gap;
  }
  return items.map((it) => ({ ...it, x: pos.get(it.id) ?? it.x }));
}

function distributeY(items: BoardItem[]): BoardItem[] {
  if (items.length < 3) return items;
  const sorted = [...items].sort((a, b) => a.y - b.y);
  const first = sorted[0];
  const last = sorted[sorted.length - 1];
  if (!first || !last) return items;
  const span = last.y + last.h - first.y;
  const gap = (span - sorted.reduce((s, it) => s + it.h, 0)) / (sorted.length - 1);
  const pos = new Map<string, number>();
  let cursor = first.y;
  for (const it of sorted) {
    pos.set(it.id, cursor);
    cursor += it.h + gap;
  }
  return items.map((it) => ({ ...it, y: pos.get(it.id) ?? it.y }));
}

// Aspect-preserving size normalization, matching PureRef. Every selected item is
// uniformly SCALED (never stretched) so a chosen dimension matches the first
// selected item's — the first selection is the size source, so the user picks
// the reference by selecting it first:
//   "w"  -> equal widths  (height follows the item's own ratio)
//   "h"  -> equal heights (width follows the item's own ratio)
//   "wh" -> equal longest side (orientation-aware Normalize Size)
// Each item keeps its own w/h ratio, so nothing is squashed.
function normalize(items: BoardItem[], dim: "w" | "h" | "wh"): BoardItem[] {
  const ref = items[0];
  if (!ref) return items;
  return items.map((it) => {
    if (it.w <= 0 || it.h <= 0) return it;
    const factor =
      dim === "w"
        ? ref.w / it.w
        : dim === "h"
          ? ref.h / it.h
          : Math.max(ref.w, ref.h) / Math.max(it.w, it.h);
    return { ...it, w: it.w * factor, h: it.h * factor };
  });
}

// Dispatches an AlignOp to its transform, returning the selected items with new
// geometry (ids preserved) for the caller to merge into the full item list.
export function applyAlign(op: AlignOp, items: BoardItem[]): BoardItem[] {
  switch (op) {
    case "left":
      return alignX(items, "min");
    case "centerH":
      return alignX(items, "center");
    case "right":
      return alignX(items, "max");
    case "top":
      return alignY(items, "min");
    case "middle":
      return alignY(items, "center");
    case "bottom":
      return alignY(items, "max");
    case "distributeH":
      return distributeX(items);
    case "distributeV":
      return distributeY(items);
    case "matchW":
      return normalize(items, "w");
    case "matchH":
      return normalize(items, "h");
    case "matchSize":
      return normalize(items, "wh");
    default: {
      const unreachable: never = op;
      return unreachable;
    }
  }
}
