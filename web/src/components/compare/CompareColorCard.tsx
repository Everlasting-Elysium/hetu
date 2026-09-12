import type { ColorResult } from "../../types";
import { DimCard, Metric, Metrics, PaletteBar, fmt2 } from "./CompareCharts";

// Color dimension card: the two weighted palettes stacked, then the ΔE00 /
// warmth / chroma metrics (issue #127). warmth_shift and chroma_diff are signed
// (target minus reference), so a leading "+" clarifies a positive drift.
const signed = (n: number): string => (n > 0 ? `+${fmt2(n)}` : fmt2(n));

export function CompareColorCard({ color }: { color: ColorResult }) {
  return (
    <DimCard title="颜色" score={color.score} testid="compare-card-color">
      <PaletteBar label="参考" palette={color.ref_palette} />
      <PaletteBar label="目标" palette={color.target_palette} />
      <Metrics>
        <Metric label="主色 ΔE" value={fmt2(color.dominant_delta_e)} />
        <Metric label="平均 ΔE" value={fmt2(color.average_delta_e)} />
        <Metric label="冷暖偏移" value={signed(color.warmth_shift)} />
        <Metric label="饱和度差" value={signed(color.chroma_diff)} />
      </Metrics>
    </DimCard>
  );
}
