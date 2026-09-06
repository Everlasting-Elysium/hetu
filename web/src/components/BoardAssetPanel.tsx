import type { Asset } from "../types";
import { thumbUrl } from "../api/client";
import { KindIcon } from "./icons";
import styles from "./BoardCanvas.module.css";

interface Props {
  assets: Asset[];
  loading: boolean;
}

// Drag source for the canvas: a scrollable strip of asset thumbnails. Each row
// carries its asset id on the native HTML5 drag payload; BoardCanvas reads it
// on drop and places a board item at the drop point.
export function BoardAssetPanel({ assets, loading }: Props) {
  return (
    <aside className={styles.panel}>
      <div className={styles.panelHead}>素材 · {assets.length}</div>
      <div className={styles.panelBody}>
        {loading && <div className={styles.panelHint}>加载中…</div>}
        {!loading && assets.length === 0 && (
          <div className={styles.panelHint}>暂无素材，先运行 scan 索引素材目录。</div>
        )}
        {assets.map((a) => {
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
