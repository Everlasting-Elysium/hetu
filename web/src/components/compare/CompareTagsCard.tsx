import type { ActionResult, ElementResult } from "../../types";
import { DimCard, Metric, Metrics, TagChips, fmt2 } from "./CompareCharts";
import styles from "./CompareCharts.module.css";

// Tag-diff card shared by the action (pose) and element (subject) dimensions:
// common/missing/extra chip groups, with the element dimension additionally
// showing its tag-Jaccard and embedding cosine (issue #127).
interface Props {
  title: string;
  testid: string;
  data: ActionResult | ElementResult;
  jaccard?: number;
  cosine?: number;
}

export function CompareTagsCard({ title, testid, data, jaccard, cosine }: Props) {
  const empty = data.common.length === 0 && data.missing.length === 0 && data.extra.length === 0;
  const hasMetrics = jaccard !== undefined || cosine !== undefined;
  return (
    <DimCard title={title} score={data.score} testid={testid}>
      {empty ? (
        <p className={styles.muted}>未检出相关标签。</p>
      ) : (
        <>
          <TagChips label="共有" tags={data.common} variant="common" />
          <TagChips label="缺失" tags={data.missing} variant="missing" />
          <TagChips label="多余" tags={data.extra} variant="extra" />
        </>
      )}
      {hasMetrics && (
        <Metrics>
          {jaccard !== undefined && <Metric label="标签 Jaccard" value={fmt2(jaccard)} />}
          {cosine !== undefined && <Metric label="向量余弦" value={fmt2(cosine)} />}
        </Metrics>
      )}
    </DimCard>
  );
}
