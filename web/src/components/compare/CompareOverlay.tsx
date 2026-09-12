import { useRef, useState } from "react";
import styles from "./Compare.module.css";
import charts from "./CompareCharts.module.css";

// Manual overlay viewer for the two staged, normalized images (issue #127). Three
// modes — side-by-side, onion-skin (opacity slider), and difference (CSS
// mix-blend-mode, NOT a pixel diff: the issue rules out auto-registration /
// per-pixel heatmaps). One shared container handles wheel-zoom + drag-pan so the
// user aligns the images by hand; nothing here computes a diff.
type Mode = "side" | "onion" | "diff";

interface Props {
  referenceUrl: string;
  targetUrl: string;
}

const MIN_SCALE = 0.5;
const MAX_SCALE = 3;
const ZOOM_STEP = 0.0015; // wheel delta → scale factor

const MODES: { id: Mode; label: string }[] = [
  { id: "side", label: "并排" },
  { id: "onion", label: "洋葱皮" },
  { id: "diff", label: "差异高亮" },
];

const clamp = (v: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, v));

export function CompareOverlay({ referenceUrl, targetUrl }: Props) {
  const [mode, setMode] = useState<Mode>("side");
  const [opacity, setOpacity] = useState(50);
  const [view, setView] = useState({ scale: 1, tx: 0, ty: 0 });
  const drag = useRef<{ x: number; y: number } | null>(null);

  const onWheel = (e: React.WheelEvent) => {
    e.preventDefault();
    setView((v) => ({ ...v, scale: clamp(v.scale - e.deltaY * ZOOM_STEP, MIN_SCALE, MAX_SCALE) }));
  };
  const onMouseDown = (e: React.MouseEvent) => {
    drag.current = { x: e.clientX - view.tx, y: e.clientY - view.ty };
  };
  const onMouseMove = (e: React.MouseEvent) => {
    if (!drag.current) return;
    setView((v) => ({ ...v, tx: e.clientX - drag.current!.x, ty: e.clientY - drag.current!.y }));
  };
  const endDrag = () => {
    drag.current = null;
  };
  const reset = () => setView({ scale: 1, tx: 0, ty: 0 });

  const transform = `translate(${view.tx}px, ${view.ty}px) scale(${view.scale})`;

  return (
    <section className={charts.card}>
      <div className={styles.overlayBar}>
        <div className={styles.tabs} data-testid="compare-overlay-mode">
          {MODES.map((m) => (
            <button
              key={m.id}
              type="button"
              className={`${styles.tab} ${mode === m.id ? styles.tabOn : ""}`}
              data-testid={`compare-mode-${m.id}`}
              aria-pressed={mode === m.id}
              onClick={() => setMode(m.id)}
            >
              {m.label}
            </button>
          ))}
        </div>
        {mode === "onion" && (
          <label className={styles.onion}>
            不透明度
            <input
              type="range"
              min={0}
              max={100}
              value={opacity}
              data-testid="compare-onion-opacity"
              onChange={(e) => setOpacity(Number(e.target.value))}
            />
            <span className={charts.mono}>{opacity}%</span>
          </label>
        )}
        <div className={styles.spacer} />
        <button
          type="button"
          className="btn btn-ghost"
          data-testid="compare-overlay-reset"
          onClick={reset}
        >
          重置视图
        </button>
      </div>

      <div
        className={styles.viewport}
        data-testid="compare-overlay"
        onWheel={onWheel}
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseUp={endDrag}
        onMouseLeave={endDrag}
      >
        {mode === "side" ? (
          <div className={styles.sideBySide} style={{ transform }}>
            <img src={referenceUrl} alt="参考" draggable={false} />
            <img src={targetUrl} alt="目标" draggable={false} />
          </div>
        ) : (
          <div className={styles.stack} style={{ transform }}>
            <img src={referenceUrl} alt="参考" draggable={false} />
            <img
              src={targetUrl}
              alt="目标"
              draggable={false}
              className={mode === "diff" ? styles.diffTop : styles.onionTop}
              style={mode === "onion" ? { opacity: opacity / 100 } : undefined}
            />
          </div>
        )}
      </div>
    </section>
  );
}
