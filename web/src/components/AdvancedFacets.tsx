import { useState } from "react";
import type { AssetShape } from "../types";
import { IconChevronRight, IconLandscape, IconPortrait, IconSquare } from "./icons";
import { MB, RangeField } from "./RangeField";
import { TimeDurationFacets, type TimeDurationFacetsProps } from "./TimeDurationFacets";
import styles from "./FilterFacets.module.css";

// Shape-facet labels + glyphs, kept exhaustive over AssetShape. SHAPES fixes the
// render order (landscape/portrait/square) without leaking Object.keys typing.
const SHAPE_LABELS: Record<AssetShape, string> = {
  landscape: "横向",
  portrait: "纵向",
  square: "方形",
};
const SHAPE_ICONS: Record<AssetShape, typeof IconLandscape> = {
  landscape: IconLandscape,
  portrait: IconPortrait,
  square: IconSquare,
};
const SHAPES: AssetShape[] = ["landscape", "portrait", "square"];

interface Props {
  activeShapes: AssetShape[];
  onToggleShape: (shape: AssetShape) => void;
  minWidth: number;
  maxWidth: number;
  minHeight: number;
  maxHeight: number;
  onSetDimensions: (minWidth: number, maxWidth: number, minHeight: number, maxHeight: number) => void;
  minSize: number;
  maxSize: number;
  onSetFileSize: (minSize: number, maxSize: number) => void;
  timeDuration: TimeDurationFacetsProps;
}

// The advanced (collapsible) filter tier: 形状 / 尺寸 / 文件大小 / 时长 / 创建时间
// / 索引时间. Split out of FilterFacets so both files stay under the LOC ceiling,
// and to shorten the sidebar: the cold facets hide behind a disclosure while the
// common tier (格式/星级/标签) stays visible. Collapsed by default, but opens on
// mount when any advanced filter is already active — a refresh/revisit must never
// hide a live filter — and the header shows an active-dimension count badge plus
// a clear-all so a collapsed-but-active tier stays discoverable. Each cold section
// reuses the same section/head/chip rhythm as the common tier so the two read as
// one filter language. Rendered by FilterFacets in the sidebar and board panel.
export function AdvancedFacets({
  activeShapes,
  onToggleShape,
  minWidth,
  maxWidth,
  minHeight,
  maxHeight,
  onSetDimensions,
  minSize,
  maxSize,
  onSetFileSize,
  timeDuration,
}: Props) {
  // One count per active dimension: shape set, any width/height bound, any size
  // bound, and each of the duration/created/indexed ranges.
  const active =
    (activeShapes.length > 0 ? 1 : 0) +
    (minWidth > 0 || maxWidth > 0 || minHeight > 0 || maxHeight > 0 ? 1 : 0) +
    (minSize > 0 || maxSize > 0 ? 1 : 0) +
    (timeDuration.minDuration > 0 || timeDuration.maxDuration > 0 ? 1 : 0) +
    (timeDuration.createdAfter > 0 || timeDuration.createdBefore > 0 ? 1 : 0) +
    (timeDuration.indexedAfter > 0 || timeDuration.indexedBefore > 0 ? 1 : 0);
  const [open, setOpen] = useState(active > 0);

  // Reset every advanced dimension in one click. Shapes clear via the same
  // per-shape toggle FilterFacets uses, the rest via their 0/0 setters.
  const clearAll = () => {
    activeShapes.forEach(onToggleShape);
    onSetDimensions(0, 0, 0, 0);
    onSetFileSize(0, 0);
    timeDuration.onSetDuration(0, 0);
    timeDuration.onSetCreatedRange(0, 0);
    timeDuration.onSetIndexedRange(0, 0);
  };

  return (
    <>
      <div className={styles.advToggle}>
        <button
          className={styles.advDisclosure}
          data-testid="advanced-toggle"
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
        >
          <IconChevronRight
            className={`${styles.advChevron} ${open ? styles.advChevronOpen : ""}`}
            width={14}
            height={14}
          />
          <span className={styles.advLabel}>高级筛选</span>
          {active > 0 && <span className={styles.advBadge}>{active}</span>}
        </button>
        {active > 0 && (
          <button className={styles.clear} aria-label="清除高级筛选" onClick={clearAll}>
            清除
          </button>
        )}
      </div>

      {open && (
        <div className={styles.advBody}>
          <div className={styles.section}>
            <div className={styles.head}>
              <span>形状</span>
              {activeShapes.length > 0 && (
                <button className={styles.clear} onClick={() => activeShapes.forEach(onToggleShape)}>
                  清除
                </button>
              )}
            </div>
            {SHAPES.map((shape) => {
              const on = activeShapes.includes(shape);
              const Icon = SHAPE_ICONS[shape];
              return (
                <button
                  key={shape}
                  className={`${styles.item} ${on ? styles.active : ""}`}
                  aria-pressed={on}
                  onClick={() => onToggleShape(shape)}
                >
                  <Icon width={15} height={15} />
                  <span className={styles.txt}>{SHAPE_LABELS[shape]}</span>
                </button>
              );
            })}
          </div>

          <div className={styles.section}>
            <div className={styles.head}>
              <span>尺寸</span>
              {(minWidth > 0 || maxWidth > 0 || minHeight > 0 || maxHeight > 0) && (
                <button className={styles.clear} onClick={() => onSetDimensions(0, 0, 0, 0)}>
                  清除
                </button>
              )}
            </div>
            <RangeField
              label="宽"
              unit="px"
              min={minWidth}
              max={maxWidth}
              onCommit={(mn, mx) => onSetDimensions(mn, mx, minHeight, maxHeight)}
            />
            <RangeField
              label="高"
              unit="px"
              min={minHeight}
              max={maxHeight}
              onCommit={(mn, mx) => onSetDimensions(minWidth, maxWidth, mn, mx)}
            />
            <div className={styles.chips}>
              <button
                className={styles.chip}
                onClick={() => onSetDimensions(minWidth, maxWidth, 1080, maxHeight)}
              >
                ≥ 1080p
              </button>
              <button
                className={styles.chip}
                onClick={() => onSetDimensions(minWidth, maxWidth, 2160, maxHeight)}
              >
                ≥ 4K
              </button>
            </div>
          </div>

          <div className={styles.section}>
            <div className={styles.head}>
              <span>文件大小</span>
              {(minSize > 0 || maxSize > 0) && (
                <button className={styles.clear} onClick={() => onSetFileSize(0, 0)}>
                  清除
                </button>
              )}
            </div>
            <RangeField
              label="大小"
              unit="MB"
              min={minSize}
              max={maxSize}
              scale={MB}
              step={0.1}
              onCommit={(mn, mx) => onSetFileSize(mn, mx)}
            />
            <div className={styles.chips}>
              <button className={styles.chip} onClick={() => onSetFileSize(0, MB)}>
                ≤ 1MB
              </button>
              <button className={styles.chip} onClick={() => onSetFileSize(MB, 10 * MB)}>
                1–10MB
              </button>
              <button className={styles.chip} onClick={() => onSetFileSize(10 * MB, 0)}>
                ≥ 10MB
              </button>
            </div>
          </div>

          <TimeDurationFacets {...timeDuration} />
        </div>
      )}
    </>
  );
}
