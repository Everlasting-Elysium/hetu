import type { AssetKind, KindCount, Tag } from "../types";
import { KindIcon } from "./icons";
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

interface Props {
  counts: KindCount[];
  activeKinds: AssetKind[];
  minRating: number;
  onToggleKind: (kind: AssetKind) => void;
  onSetRating: (rating: number) => void;
  // Optional tag facet: the board asset panel passes these to get a labeled
  // 标签 section consistent with 格式/星级. The main sidebar omits them because it
  // renders its own CRUD-capable tag list, so no 标签 section appears there.
  tags?: Tag[];
  activeTag?: string | null;
  onPickTag?: (tagId: string) => void;
}

// Shared tag + format + star facets, styled like the sidebar rows so they read
// as one filter language. Tags render as toggleable chips (single-select, clears
// on re-pick); formats with no live assets in the current scope are hidden and
// toggle (multi-select); the star row reuses RatingStars as a "≥ N stars"
// selector. Mounted in the main sidebar (format/star) and the board asset panel
// (tag/format/star) — issue #75.
export function FilterFacets({
  counts,
  activeKinds,
  minRating,
  onToggleKind,
  onSetRating,
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
    </>
  );
}
