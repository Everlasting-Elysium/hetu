import type { CompareResponse } from "../../types";
import { fmt, Unavailable } from "./CompareCharts";
import { CompareColorCard } from "./CompareColorCard";
import { CompareToneCard } from "./CompareToneCard";
import { CompareLightingCard } from "./CompareLightingCard";
import { CompareTagsCard } from "./CompareTagsCard";
import { dimLabel } from "./labels";
import styles from "./Compare.module.css";
import charts from "./CompareCharts.module.css";

// The per-dimension result grid (issue #127): a prominent overall score with the
// scored-dimension tags, then one card per dimension. The three pixel dimensions
// (color/tone/lighting) are always computed; action/element come back undefined
// when no Tagger/Embedder is configured, so they fall back to an Unavailable card
// rather than a blank or an error.
export function CompareCards({ result }: { result: CompareResponse }) {
  return (
    <div className={styles.cards} data-testid="compare-cards">
      <section className={`${charts.card} ${styles.overall}`} data-testid="compare-overall">
        <div className={styles.overallScore}>{fmt(result.overall_score)}</div>
        <div className={styles.overallLabel}>总分</div>
        <div className={styles.dimTags}>
          {result.dimensions.map((d) => (
            <span key={d} className={styles.dimTag}>
              {dimLabel(d)}
            </span>
          ))}
        </div>
      </section>

      {result.color && <CompareColorCard color={result.color} />}
      {result.tone && <CompareToneCard tone={result.tone} />}
      {result.lighting && <CompareLightingCard lighting={result.lighting} />}

      {result.action ? (
        <CompareTagsCard title="动作" testid="compare-card-action" data={result.action} />
      ) : (
        <Unavailable title="动作" testid="compare-card-action" />
      )}

      {result.element ? (
        <CompareTagsCard
          title="元素"
          testid="compare-card-element"
          data={result.element}
          jaccard={result.element.tag_jaccard}
          cosine={result.element.cosine}
        />
      ) : (
        <Unavailable title="元素" testid="compare-card-element" />
      )}
    </div>
  );
}
