import type { AssetKind, KindCount } from "../types";
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
}

// Shared format + star facets, styled like the sidebar tag rows so the two read
// as one filter language. Formats with no live assets in the current scope are
// hidden; clicking a format toggles it (multi-select), and the star row reuses
// RatingStars as a "≥ N stars" selector (clicking the active star clears it).
// Mounted in both the main sidebar and the board asset panel (issue #75).
export function FilterFacets({ counts, activeKinds, minRating, onToggleKind, onSetRating }: Props) {
  const visible = counts.filter((c) => c.count > 0);

  return (
    <>
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
