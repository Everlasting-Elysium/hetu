import { useEffect, useState } from "react";
import type { Asset, AssetKind, KindCount, Query, Tag } from "../types";
import { thumbUrl } from "../api/client";
import { FilterFacets } from "./FilterFacets";
import { IconClose, IconSearch, KindIcon } from "./icons";
import styles from "./BoardCanvas.module.css";

interface Props {
  assets: Asset[];
  loading: boolean;
  tags: Tag[];
  query: Query;
  kindCounts: KindCount[];
  onKeyword: (keyword: string) => void;
  onPickTag: (tagId: string) => void;
  onToggleKind: (kind: AssetKind) => void;
  onSetRating: (rating: number) => void;
}

// Drag source for the canvas: a search box + the shared tag/format/star facets
// on top of a scrollable strip of asset thumbnails. The panel owns only the
// search input's local text (debounced to onKeyword); every other filter is the
// board query, resolved server-side by the same useAssets/useFacets the main
// library uses (issue #75). Each row carries its asset id on the native drag
// payload; BoardCanvas reads it on drop.
export function BoardAssetPanel(p: Props) {
  const [text, setText] = useState("");

  useEffect(() => {
    const t = setTimeout(() => p.onKeyword(text), 300);
    return () => clearTimeout(t);
  }, [text, p.onKeyword]);

  return (
    <aside className={styles.panel}>
      <div className={styles.panelFilters}>
        <div className={styles.panelSearch}>
          <IconSearch width={14} height={14} />
          <input
            className={`input ${styles.searchField}`}
            placeholder="搜索素材…"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          {text && (
            <button className={styles.searchClear} title="清除" onClick={() => setText("")}>
              <IconClose width={13} height={13} />
            </button>
          )}
        </div>

        {p.tags.length > 0 && (
          <div className={styles.tagChips}>
            {p.tags.map((t) => (
              <button
                key={t.id}
                className={`${styles.chip} ${p.query.tagId === t.id ? styles.chipOn : ""}`}
                onClick={() => p.onPickTag(t.id)}
              >
                {t.color && <i className={styles.chipDot} style={{ background: t.color }} />}
                {t.name}
              </button>
            ))}
          </div>
        )}

        <FilterFacets
          counts={p.kindCounts}
          activeKinds={p.query.kind}
          minRating={p.query.minRating}
          onToggleKind={p.onToggleKind}
          onSetRating={p.onSetRating}
        />
      </div>

      <div className={styles.panelHead}>素材 · {p.assets.length}</div>
      <div className={styles.panelBody}>
        {p.loading && <div className={styles.panelHint}>加载中…</div>}
        {!p.loading && p.assets.length === 0 && (
          <div className={styles.panelHint}>没有匹配的素材，调整搜索或筛选试试。</div>
        )}
        {p.assets.map((a) => {
          const label = a.display_name || a.name;
          return (
            <div
              key={a.id}
              className={styles.source}
              title={`拖拽到画布：${label}`}
              draggable
              onDragStart={(e) => {
                e.dataTransfer.setData("text/plain", a.id);
                e.dataTransfer.effectAllowed = "copy";
              }}
            >
              <div className={styles.sourceThumb}>
                {a.thumb ? (
                  <img src={thumbUrl(a.id)} alt="" loading="lazy" draggable={false} />
                ) : (
                  <KindIcon kind={a.kind} width={22} height={22} />
                )}
              </div>
              <span className={styles.sourceName}>{label}</span>
            </div>
          );
        })}
      </div>
    </aside>
  );
}
