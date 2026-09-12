import type { UseCompare } from "../../hooks/useCompare";
import { IconArrowLeft, IconSwap } from "../icons";
import { CompareSlot } from "./CompareSlot";
import { CompareCards } from "./CompareCards";
import { CompareOverlay } from "./CompareOverlay";
import { CompareCritique } from "./CompareCritique";
import styles from "./Compare.module.css";

// The two-image comparison page (issue #127), a transient full-area view entered
// from the batch bar when exactly two assets are selected. It renders the two
// slots (each swappable for a local upload), a run button, and — once a result
// arrives — the per-dimension cards, the manual overlay viewer, and the AI
// critique panel. All comparison state lives in the shared useCompare hook so the
// batch-bar entry point can seed it before switching the view.
interface Props {
  cmp: UseCompare;
  onError: (msg: string) => void;
  onExit: () => void;
}

export function ComparePage({ cmp, onError, onExit }: Props) {
  const { reference, target, result, loading } = cmp;
  const ready = reference !== null && target !== null;

  return (
    <div className={styles.page} data-testid="compare-page">
      <header className={styles.pageHead}>
        <button type="button" className="btn btn-ghost" onClick={onExit}>
          <IconArrowLeft width={14} height={14} /> 返回
        </button>
        <h2 className={styles.pageTitle}>图像对比</h2>
        <div className={styles.spacer} />
        <button
          type="button"
          className="btn btn-ghost"
          data-testid="compare-swap"
          disabled={!ready}
          onClick={cmp.swap}
        >
          <IconSwap width={14} height={14} /> 互换
        </button>
      </header>

      <div className={styles.slots}>
        <CompareSlot
          label="参考图（临摹/仿拍对象）"
          slot={reference}
          onFile={(f) => cmp.setFile("reference", f)}
          onReturnToLibrary={onExit}
          testid="compare-slot-reference"
        />
        <CompareSlot
          label="目标图（你的作品）"
          slot={target}
          onFile={(f) => cmp.setFile("target", f)}
          onReturnToLibrary={onExit}
          testid="compare-slot-target"
        />
      </div>

      <div className={styles.runRow}>
        <button
          type="button"
          className="btn btn-primary"
          data-testid="compare-run"
          disabled={!ready || loading}
          onClick={() => void cmp.run(onError)}
        >
          {loading ? "对比中…" : "开始对比"}
        </button>
      </div>

      {result && (
        <div className={styles.result}>
          <CompareCards result={result} />
          <CompareOverlay
            referenceUrl={result.overlays.reference_url}
            targetUrl={result.overlays.target_url}
          />
          <CompareCritique critique={result.critique} />
        </div>
      )}
    </div>
  );
}
