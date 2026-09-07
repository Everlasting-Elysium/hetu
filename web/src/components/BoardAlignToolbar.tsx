import { Fragment, type ReactNode } from "react";
import type { BoardItem } from "../types";
import { type AlignOp, applyAlign } from "./boardAlign";
import styles from "./BoardCanvas.module.css";

interface Props {
  selectedItems: BoardItem[];
  onUpdate: (updates: BoardItem[]) => void;
}

const svg = (children: ReactNode): ReactNode => (
  <svg width={16} height={16} viewBox="0 0 24 24" fill="currentColor">
    {children}
  </svg>
);

// Each glyph reads as alignment bars: a thin reference edge plus two content
// bars snapped to it (edge/center ops), evenly spaced bars (distribute), or two
// boxes sharing a dimension (match). All filled, so intent reads at 16px.
const ICONS: Record<AlignOp, ReactNode> = {
  left: svg(
    <>
      <rect x={2} y={3} width={1.8} height={18} rx={0.6} />
      <rect x={6} y={5} width={14} height={4} rx={1} />
      <rect x={6} y={15} width={9} height={4} rx={1} />
    </>,
  ),
  centerH: svg(
    <>
      <rect x={11.1} y={3} width={1.8} height={18} rx={0.6} />
      <rect x={5} y={5} width={14} height={4} rx={1} />
      <rect x={7.5} y={15} width={9} height={4} rx={1} />
    </>,
  ),
  right: svg(
    <>
      <rect x={20.2} y={3} width={1.8} height={18} rx={0.6} />
      <rect x={6} y={5} width={14} height={4} rx={1} />
      <rect x={11} y={15} width={9} height={4} rx={1} />
    </>,
  ),
  top: svg(
    <>
      <rect x={3} y={2} width={18} height={1.8} rx={0.6} />
      <rect x={5} y={6} width={4} height={14} rx={1} />
      <rect x={15} y={6} width={4} height={9} rx={1} />
    </>,
  ),
  middle: svg(
    <>
      <rect x={3} y={11.1} width={18} height={1.8} rx={0.6} />
      <rect x={5} y={5} width={4} height={14} rx={1} />
      <rect x={15} y={7.5} width={4} height={9} rx={1} />
    </>,
  ),
  bottom: svg(
    <>
      <rect x={3} y={20.2} width={18} height={1.8} rx={0.6} />
      <rect x={5} y={6} width={4} height={14} rx={1} />
      <rect x={15} y={11} width={4} height={9} rx={1} />
    </>,
  ),
  distributeH: svg(
    <>
      <rect x={3} y={5} width={4} height={14} rx={1} />
      <rect x={10} y={5} width={4} height={14} rx={1} />
      <rect x={17} y={5} width={4} height={14} rx={1} />
    </>,
  ),
  distributeV: svg(
    <>
      <rect x={5} y={3} width={14} height={4} rx={1} />
      <rect x={5} y={10} width={14} height={4} rx={1} />
      <rect x={5} y={17} width={14} height={4} rx={1} />
    </>,
  ),
  matchW: svg(
    <>
      <rect x={5} y={4} width={14} height={6} rx={1} />
      <rect x={5} y={14} width={14} height={6} rx={1} />
    </>,
  ),
  matchH: svg(
    <>
      <rect x={3} y={6} width={8} height={12} rx={1} />
      <rect x={13} y={6} width={8} height={12} rx={1} />
    </>,
  ),
  matchSize: svg(
    <>
      <rect x={4} y={5} width={7} height={14} rx={1} />
      <rect x={13} y={5} width={7} height={14} rx={1} />
    </>,
  ),
};

// Platform-aware modifier glyphs for the shortcut hints, mirroring PureRef.
const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);
const MOD = isMac ? "⌘" : "Ctrl+";
const ALT = isMac ? "⌥" : "Alt+";

// The three match ops scale aspect-preserving (never stretch), so the labels say
// so — this is what the user asked for over the old squashing behavior.
const LABELS: Record<AlignOp, string> = {
  left: "左对齐",
  centerH: "水平居中",
  right: "右对齐",
  top: "顶对齐",
  middle: "垂直居中",
  bottom: "底对齐",
  distributeH: "水平分布",
  distributeV: "垂直分布",
  matchW: "等宽（保持比例）",
  matchH: "等高（保持比例）",
  matchSize: "等尺寸（最长边·保持比例）",
};

// Shortcut hint per op (empty = no keyboard binding); mirrors useArrangeShortcuts.
const KEYS: Record<AlignOp, string> = {
  left: `${MOD}←`,
  centerH: "",
  right: `${MOD}→`,
  top: `${MOD}↑`,
  middle: "",
  bottom: `${MOD}↓`,
  distributeH: `${MOD}${ALT}⇧↑`,
  distributeV: `${MOD}${ALT}⇧↓`,
  matchW: `${MOD}${ALT}→`,
  matchH: `${MOD}${ALT}←`,
  matchSize: `${MOD}${ALT}↑`,
};

const titleFor = (op: AlignOp): string => (KEYS[op] ? `${LABELS[op]} (${KEYS[op]})` : LABELS[op]);

// Three visual groups separated by hairlines: align, distribute, match.
const GROUPS: AlignOp[][] = [
  ["left", "centerH", "right", "top", "middle", "bottom"],
  ["distributeH", "distributeV"],
  ["matchW", "matchH", "matchSize"],
];

// The floating arrange bar shown above the canvas whenever two or more items
// are selected (BoardCanvas gates on that). Every button routes a selection
// through applyAlign and hands the new geometry back via onUpdate.
export function BoardAlignToolbar({ selectedItems, onUpdate }: Props) {
  return (
    <div className={styles.alignBar} data-testid="align-bar">
      {GROUPS.map((group, gi) => (
        <Fragment key={group.join("-")}>
          {gi > 0 && <span className={styles.alignSep} />}
          {group.map((op) => (
            <button
              key={op}
              type="button"
              className={styles.alignBtn}
              title={titleFor(op)}
              aria-label={titleFor(op)}
              onClick={() => onUpdate(applyAlign(op, selectedItems))}
            >
              {ICONS[op]}
            </button>
          ))}
        </Fragment>
      ))}
      <span className={styles.alignHint}>
        {MOD}方向 对齐 · {MOD}{ALT}方向 等尺寸 · {MOD}{ALT}⇧方向 分布
      </span>
    </div>
  );
}
