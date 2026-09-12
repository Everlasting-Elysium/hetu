import type { CritiqueResult } from "../../types";
import { dimLabel } from "./labels";
import styles from "./Compare.module.css";
import charts from "./CompareCharts.module.css";

// AI critique panel (issue #127). When a VisionCritic/VLM is configured the
// backend returns available=true with an overall summary, per-dimension notes,
// and the producing model. When it is not, available=false and this renders a
// clearly-distinct guidance box (never a blank or an error) telling the user how
// to enable it.
export function CompareCritique({ critique }: { critique: CritiqueResult }) {
  if (!critique.available) {
    return (
      <section
        className={`${charts.card} ${styles.critiqueOff}`}
        data-testid="compare-critique-unavailable"
      >
        <h3 className={charts.cardTitle}>AI 点评</h3>
        <p className={charts.muted}>
          未启用 AI 点评。设置 <code className={charts.mono}>HETU_AI_VLM_MODEL</code>{" "}
          环境变量后即可获得逐维度可执行建议。
        </p>
      </section>
    );
  }

  const dims = critique.dimensions ?? {};
  const notes = Object.entries(dims);
  return (
    <section className={charts.card} data-testid="compare-critique">
      <h3 className={charts.cardTitle}>AI 点评</h3>
      {critique.summary && <p className={styles.critiqueSummary}>{critique.summary}</p>}
      {notes.length > 0 && (
        <ul className={styles.critiqueList}>
          {notes.map(([k, v]) => (
            <li key={k}>
              <b>{dimLabel(k)}</b>：{v}
            </li>
          ))}
        </ul>
      )}
      {critique.model && <div className={styles.critiqueModel}>模型：{critique.model}</div>}
    </section>
  );
}
