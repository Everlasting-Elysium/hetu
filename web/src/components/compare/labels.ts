// Chinese display labels for the compare dimension keys and enum values, kept in
// one place so every card and the AI critique render them identically. The keys
// mirror the Go json values (imgcompare.Dimension, ToneKey, LightDir).

export const DIM_LABELS: Record<string, string> = {
  color: "颜色",
  tone: "明暗",
  lighting: "光影",
  action: "动作",
  element: "元素",
};

export const KEY_LABELS: Record<string, string> = {
  high: "高调",
  mid: "中调",
  low: "低调",
};

export const LIGHT_LABELS: Record<string, string> = {
  "top-left": "左上",
  "top-right": "右上",
  "bottom-left": "左下",
  "bottom-right": "右下",
  "center-even": "均匀",
};

export const dimLabel = (key: string): string => DIM_LABELS[key] ?? key;
export const keyLabel = (key: string): string => KEY_LABELS[key] ?? key;
export const lightLabel = (key: string): string => LIGHT_LABELS[key] ?? key;
