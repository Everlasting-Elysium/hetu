import { useEffect, useState } from "react";
import type { Asset } from "../types";
import { thumbUrl } from "../api/client";
import { IconClose, IconSearch, KindIcon } from "./icons";
import styles from "./BoardCanvas.module.css";

interface Props {
  assets: Asset[];
  loading: boolean;
  onKeyword: (keyword: string) => void;
}

// Drag source for the canvas: a search box on top of a scrollable strip of
// asset thumbnails. Folder/tag/format/star filtering moved to the global
// sidebar (issue #108) — it drives the board-local query BoardCanvas passes
// down as `assets`, so this panel only owns the search input's local text
// (debounced to onKeyword). Each row carries its asset id on the native drag
// payload; BoardCanvas reads it on drop.
export function BoardAssetPanel(p: Props) {
  const [text, setText] = useState("");

  useEffect(() => {
    const t = setTimeout(() => p.onKeyword(text), 300);
    return () => clearTimeout(t);
  }, [text, p.onKeyword]);

  return (
    <aside className={styles.panel}>
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
