import type { LightingResult } from "../../types";
import { DimCard, Metric, Metrics, ZoneBar, fmt2 } from "./CompareCharts";
import { lightLabel } from "./labels";

// Lighting (光影) card: the two shadow/midtone/highlight zone bars, then the
// per-zone diffs, contrast, and coarse light direction for each side (#127).
export function CompareLightingCard({ lighting: l }: { lighting: LightingResult }) {
  return (
    <DimCard title="光影" score={l.score} testid="compare-card-lighting">
      <ZoneBar label="参考" zones={l.ref_zones} />
      <ZoneBar label="目标" zones={l.target_zones} />
      <Metrics>
        <Metric label="暗部差" value={fmt2(l.zone_diff.shadow)} />
        <Metric label="中间调差" value={fmt2(l.zone_diff.midtone)} />
        <Metric label="高光差" value={fmt2(l.zone_diff.highlight)} />
        <Metric label="参考对比度" value={fmt2(l.ref_contrast)} />
        <Metric label="目标对比度" value={fmt2(l.target_contrast)} />
        <Metric label="参考光向" value={lightLabel(l.ref_light)} />
        <Metric label="目标光向" value={lightLabel(l.target_light)} />
        <Metric label="光向一致" value={l.light_matches ? "是" : "否"} />
      </Metrics>
    </DimCard>
  );
}
