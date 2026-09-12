import type { ToneResult } from "../../types";
import { DimCard, Histogram, Metric, Metrics, fmt2 } from "./CompareCharts";
import { keyLabel } from "./labels";

// Tone (明暗) card: the two 32-bucket luma histograms, then the brightness stats
// for each side plus the histogram intersection driving the score (issue #127).
export function CompareToneCard({ tone }: { tone: ToneResult }) {
  const r = tone.ref_stats;
  const t = tone.target_stats;
  return (
    <DimCard title="明暗" score={tone.score} testid="compare-card-tone">
      <Histogram label="参考" bins={r.histogram} />
      <Histogram label="目标" bins={t.histogram} />
      <Metrics>
        <Metric label="直方图交集" value={fmt2(tone.hist_intersection)} />
        <Metric label="参考均值" value={fmt2(r.mean)} />
        <Metric label="目标均值" value={fmt2(t.mean)} />
        <Metric label="参考中位" value={fmt2(r.median)} />
        <Metric label="目标中位" value={fmt2(t.median)} />
        <Metric label="参考对比度" value={fmt2(r.std_dev)} />
        <Metric label="目标对比度" value={fmt2(t.std_dev)} />
        <Metric label="参考动态范围" value={fmt2(r.dynamic_range)} />
        <Metric label="目标动态范围" value={fmt2(t.dynamic_range)} />
        <Metric label="参考影调" value={keyLabel(r.key)} />
        <Metric label="目标影调" value={keyLabel(t.key)} />
      </Metrics>
    </DimCard>
  );
}
