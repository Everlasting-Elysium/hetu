import { useEffect } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Asset } from "../types";
import type { Selection } from "../hooks/useSelection";
import { AssetCard } from "./AssetCard";
import { GridEmpty, GridError, GridSpinner } from "./GridStates";
import {
  GAP,
  GRID_COL_MIN,
  META_H,
  PAD_Y,
  chunk,
  columnMetrics,
  useContainerWidth,
} from "./gridLayout";
import styles from "./AssetGrid.module.css";

interface Props {
  assets: Asset[];
  loading: boolean;
  error: string | null;
  selection: Selection;
  emptyHint: string;
  onRate: (id: string, rating: number) => void;
  onColor: (id: string, hex: string) => void;
  onDetail: (id: string) => void;
}

// Uniform square-thumb grid, virtualized by row so 10k+ assets stay smooth.
// Columns are derived from the measured width; each row is a CSS grid.
export function AssetGrid({
  assets,
  loading,
  error,
  selection,
  emptyHint,
  onRate,
  onColor,
  onDetail,
}: Props) {
  const [scrollRef, width] = useContainerWidth<HTMLDivElement>();
  const { columns, columnWidth } = columnMetrics(width, GRID_COL_MIN);
  const rowHeight = columnWidth + META_H;
  const rows = chunk(assets, columns);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    gap: GAP,
    overscan: 4,
  });

  // Re-measure when column geometry changes (viewport resize).
  useEffect(() => {
    virtualizer.measure();
  }, [columns, rowHeight, virtualizer]);

  const busy = loading && assets.length === 0;

  return (
    <div ref={scrollRef} className={styles.scroll} data-testid="grid-view">
      {busy ? (
        <GridSpinner />
      ) : error ? (
        <GridError message={error} />
      ) : assets.length === 0 ? (
        <GridEmpty hint={emptyHint} />
      ) : (
        <div className={styles.sizer} style={{ height: virtualizer.getTotalSize() + PAD_Y * 2 }}>
          {virtualizer.getVirtualItems().map((vRow) => {
            const row = rows[vRow.index] ?? [];
            return (
              <div
                key={vRow.key}
                className={styles.row}
                style={{
                  transform: `translateY(${vRow.start + PAD_Y}px)`,
                  gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
                }}
              >
                {row.map((a) => (
                  <AssetCard
                    key={a.id}
                    asset={a}
                    selected={selection.isSelected(a.id)}
                    onSelect={(e) => selection.select(a.id, e)}
                    onToggleCheck={() => selection.toggle(a.id)}
                    onRate={(r) => onRate(a.id, r)}
                    onColor={(hex) => onColor(a.id, hex)}
                    onDetail={() => onDetail(a.id)}
                  />
                ))}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
