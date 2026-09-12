import type { ReactNode } from "react";
import type { Swatch } from "../../types";
import styles from "./CompareCharts.module.css";

// Small, reusable presentational primitives shared by every dimension card, so
// each card file stays a thin data-to-props mapping. No dimension logic lives
// here — only the charts and layout shells (issue #127).

// 0-100 score formatted to one decimal; other metrics use fmt2 (two decimals).
export const fmt = (n: number): string =>
  Number.isFinite(n) ? String(Math.round(n * 10) / 10) : "0";
export const fmt2 = (n: number): string =>
  Number.isFinite(n) ? (Math.round(n * 100) / 100).toFixed(2) : "0";

// A dimension card shell: title + a 0-100 score badge + body.
export function DimCard(props: {
  title: string;
  score: number;
  testid: string;
  children: ReactNode;
}) {
  return (
    <section className={styles.card} data-testid={props.testid}>
      <div className={styles.cardHead}>
        <h3 className={styles.cardTitle}>{props.title}</h3>
        <span className={styles.scoreBadge}>{fmt(props.score)}</span>
      </div>
      {props.children}
    </section>
  );
}

// The "dimension not computed" placeholder — action/element with no AI backend
// configured come back undefined; this keeps the card grid intact (never blank).
export function Unavailable({ title, testid }: { title: string; testid: string }) {
  return (
    <section className={`${styles.card} ${styles.cardMuted}`} data-testid={testid}>
      <div className={styles.cardHead}>
        <h3 className={styles.cardTitle}>{title}</h3>
      </div>
      <p className={styles.muted}>此维度因 AI 未配置而不可用。</p>
    </section>
  );
}

export function Metrics({ children }: { children: ReactNode }) {
  return <div className={styles.metrics}>{children}</div>;
}

export function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.metric}>
      <span className={styles.metricLabel}>{label}</span>
      <span className={styles.metricValue}>{value}</span>
    </div>
  );
}

// Weighted palette strip — the read-only strip shape from PaletteEditor, each
// swatch sized by its coverage weight. A floor keeps tiny swatches clickable.
export function PaletteBar({ label, palette }: { label: string; palette: Swatch[] }) {
  return (
    <div className={styles.chartRow}>
      <span className={styles.rowLabel}>{label}</span>
      <div className={styles.palette}>
        {palette.map((s, i) => (
          <span
            key={`${s.hex}-${i}`}
            className={styles.paletteSwatch}
            title={`${s.hex} · ${(s.weight * 100).toFixed(0)}%`}
            style={{ background: s.hex, flex: Math.max(s.weight, 0.02) }}
          />
        ))}
      </div>
    </div>
  );
}

// 32-bucket luma histogram as CSS bars, each normalized to the row's own peak.
export function Histogram({ label, bins }: { label: string; bins: number[] }) {
  const max = Math.max(...bins, 1e-6);
  return (
    <div className={styles.chartRow}>
      <span className={styles.rowLabel}>{label}</span>
      <div className={styles.histogram}>
        {bins.map((b, i) => (
          <span key={i} className={styles.histBar} style={{ height: `${(b / max) * 100}%` }} />
        ))}
      </div>
    </div>
  );
}

// shadow/midtone/highlight stacked proportion bar (three segments sized by value).
export function ZoneBar({
  label,
  zones,
}: {
  label: string;
  zones: { shadow: number; midtone: number; highlight: number };
}) {
  return (
    <div className={styles.chartRow}>
      <span className={styles.rowLabel}>{label}</span>
      <div className={styles.zoneBar}>
        <span className={styles.zoneShadow} style={{ flex: Math.max(zones.shadow, 0.001) }} />
        <span className={styles.zoneMid} style={{ flex: Math.max(zones.midtone, 0.001) }} />
        <span className={styles.zoneHigh} style={{ flex: Math.max(zones.highlight, 0.001) }} />
      </div>
    </div>
  );
}

// A labelled group of tag chips (共有/缺失/多余). Empty groups render nothing.
export function TagChips({
  label,
  tags,
  variant,
}: {
  label: string;
  tags: string[];
  variant: "common" | "missing" | "extra";
}) {
  if (tags.length === 0) return null;
  const cls =
    variant === "common" ? styles.chipCommon : variant === "missing" ? styles.chipMissing : styles.chipExtra;
  return (
    <div className={styles.chipGroup}>
      <span className={styles.rowLabel}>{label}</span>
      <div className={styles.chips}>
        {tags.map((t) => (
          <span key={t} className={`${styles.chip} ${cls}`}>
            {t}
          </span>
        ))}
      </div>
    </div>
  );
}
