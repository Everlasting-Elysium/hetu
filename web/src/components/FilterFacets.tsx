import type { AssetKind, AssetShape, KindCount, Tag } from "../types";
import { IconLandscape, IconPortrait, IconSquare, KindIcon } from "./icons";
import { MB, RangeField } from "./RangeField";
import { RatingStars } from "./RatingStars";
import styles from "./FilterFacets.module.css";

// Human labels for the format facet. Keyed by AssetKind so the map stays
// exhaustive as kinds are added (matches domain.AllKinds order via the API).
const KIND_LABELS: Record<AssetKind, string> = {
  image: "图片",
  video: "视频",
  audio: "音频",
  model: "模型",
  document: "文档",
  other: "其他",
};

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
  counts: KindCount[];
  activeKinds: AssetKind[];
  minRating: number;
  onToggleKind: (kind: AssetKind) => void;
  onSetRating: (rating: number) => void;
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
  // Optional tag facet: the board asset panel passes these to get a labeled
  // 标签 section consistent with 格式/星级. The main sidebar omits them because it
  // renders its own CRUD-capable tag list, so no 标签 section appears there.
  tags?: Tag[];
  activeTag?: string | null;
  onPickTag?: (tagId: string) => void;
}

// Shared tag + format + shape + star + dimension + size facets, styled like the
// sidebar rows so they read as one filter language. Tags render as toggleable
// chips (single-select, clears on re-pick); formats with no live assets in the
// current scope are hidden and toggle (multi-select); shapes toggle like formats
// (multi-select); the star row reuses RatingStars as a "≥ N stars" selector; the
// 尺寸 (宽/高) and 文件大小 rows are min–max numeric ranges. File size shows MB but
// commits bytes, so that conversion stays inside RangeField and the query layer
// only ever holds bytes (#101). Mounted in the main sidebar and the board asset
// panel (issue #75).
export function FilterFacets({
  counts,
  activeKinds,
  minRating,
  onToggleKind,
  onSetRating,
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
  tags,
  activeTag,
  onPickTag,
}: Props) {
  const visible = counts.filter((c) => c.count > 0);

  return (
    <>
      {onPickTag && tags && tags.length > 0 && (
        <div className={styles.section}>
          <div className={styles.head}>
            <span>标签</span>
            {activeTag && (
              <button className={styles.clear} onClick={() => onPickTag(activeTag)}>
                清除
              </button>
            )}
          </div>
          <div className={styles.chips}>
            {tags.map((t) => (
              <button
                key={t.id}
                className={`${styles.chip} ${activeTag === t.id ? styles.chipOn : ""}`}
                aria-pressed={activeTag === t.id}
                onClick={() => onPickTag(t.id)}
              >
                {t.color && <i className={styles.chipDot} style={{ background: t.color }} />}
                {t.name}
              </button>
            ))}
          </div>
        </div>
      )}

      <div className={styles.section}>
        <div className={styles.head}>
          <span>格式</span>
          {activeKinds.length > 0 && (
            <button className={styles.clear} onClick={() => activeKinds.forEach(onToggleKind)}>
              清除
            </button>
          )}
        </div>
        {visible.length === 0 ? (
          <div className={styles.empty}>暂无素材</div>
        ) : (
          visible.map((c) => {
            const on = activeKinds.includes(c.kind);
            return (
              <button
                key={c.kind}
                className={`${styles.item} ${on ? styles.active : ""}`}
                aria-pressed={on}
                onClick={() => onToggleKind(c.kind)}
              >
                <KindIcon kind={c.kind} width={15} height={15} />
                <span className={styles.txt}>{KIND_LABELS[c.kind]}</span>
                <span className={styles.count}>{c.count}</span>
              </button>
            );
          })
        )}
      </div>

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
          <span>星级</span>
          {minRating > 0 && (
            <button className={styles.clear} onClick={() => onSetRating(0)}>
              清除
            </button>
          )}
        </div>
        <div className={styles.rating}>
          <RatingStars value={minRating} size={16} onChange={onSetRating} />
          <span className={styles.ratingLabel}>{minRating > 0 ? `${minRating} 星以上` : "全部星级"}</span>
        </div>
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
    </>
  );
}
