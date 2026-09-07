import { useState } from "react";
import type { ContentRect } from "./boardExport";
import styles from "./BoardCanvas.module.css";

interface Props {
  contentSize: ContentRect | null;
  onExport: (pixelRatio: number) => void;
  onClose: () => void;
}

// Resolution multipliers over the board's natural (1x) content size.
const PRESETS = [1, 2, 4];
const near = (a: number, b: number): boolean => Math.abs(a - b) < 0.001;

// Chooses the export resolution and fires the download. A single `ratio` drives
// everything: presets set it directly, and the width/height fields stay
// aspect-locked by mapping an entered pixel size back onto the same ratio, so
// the PNG is always a uniform scale of the content box (boardExport crops it).
export function BoardExportDialog({ contentSize, onExport, onClose }: Props) {
  const [ratio, setRatio] = useState(2);
  const empty = !contentSize || contentSize.w <= 0 || contentSize.h <= 0;
  const w = contentSize ? Math.round(contentSize.w * ratio) : 0;
  const h = contentSize ? Math.round(contentSize.h * ratio) : 0;

  const setFromWidth = (px: number) => {
    if (contentSize && contentSize.w > 0 && px > 0) setRatio(px / contentSize.w);
  };
  const setFromHeight = (px: number) => {
    if (contentSize && contentSize.h > 0 && px > 0) setRatio(px / contentSize.h);
  };

  return (
    <div className={styles.dialogBackdrop} onMouseDown={onClose}>
      <div
        className={styles.dialog}
        role="dialog"
        aria-label="导出图板"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className={styles.dialogTitle}>导出图板</div>
        {empty ? (
          <p className={styles.dialogHint}>画布为空，添加素材后再导出。</p>
        ) : (
          <>
            <div className={styles.presetRow}>
              {PRESETS.map((p) => (
                <button
                  key={p}
                  type="button"
                  className={`${styles.preset} ${near(ratio, p) ? styles.presetActive : ""}`}
                  onClick={() => setRatio(p)}
                >
                  {p}x
                </button>
              ))}
            </div>
            <div className={styles.sizeRow}>
              <label className={styles.sizeField}>
                宽
                <input
                  className="input"
                  type="number"
                  min={1}
                  value={w}
                  onChange={(e) => setFromWidth(Number(e.target.value))}
                />
              </label>
              <span className={styles.sizeX}>×</span>
              <label className={styles.sizeField}>
                高
                <input
                  className="input"
                  type="number"
                  min={1}
                  value={h}
                  onChange={(e) => setFromHeight(Number(e.target.value))}
                />
              </label>
              <span className={styles.sizeUnit}>px</span>
            </div>
          </>
        )}
        <div className={styles.dialogActions}>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            取消
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={empty}
            onClick={() => onExport(ratio)}
          >
            导出 PNG
          </button>
        </div>
      </div>
    </div>
  );
}
