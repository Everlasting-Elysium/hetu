import { useEffect } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Asset } from "../types";
import type { Selection } from "../hooks/useSelection";
import { AssetCard } from "./AssetCard";
import { GridEmpty, GridError, GridSpinner } from "./GridStates";
import {
  GAP,
  PAD_X,
  PAD_Y,
  WATERFALL_COL_TARGET,
  columnMetrics,
  useContainerWidth,
  waterfallItem,
} from "./gridLayout";
import grid from "./AssetGrid.module.css";
import styles from "./WaterfallGrid.module.css";

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

// Masonry: equal-width columns, per-item height from the asset's aspect ratio.
// react-virtual `lanes` packs each item into the shortest column by estimate, so
// with exact aspect-derived estimates the layout is stable (no measure, no shift).
export function WaterfallGrid({
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
  const { columns, columnWidth } = columnMetrics(width, WATERFALL_COL_TARGET);

  const virtualizer = useVirtualizer({
    count: assets.length,
    lanes: columns,
    getScrollElement: () => scrollRef.current,
    estimateSize: (i) => {
      const a = assets[i];
      return a ? waterfallItem(a.width, a.height, columnWidth).height : columnWidth;
    },
    gap: GAP,
    overscan: 6,
  });

  // Re-pack on resize AND on dataset change: estimateSize depends on each asset's
  // aspect ratio, but the virtualizer only re-derives estimates via measure(),
  // which nothing else triggers for a same-count swap (would leave stale positions).
  useEffect(() => {
    virtualizer.measure();
  }, [columns, columnWidth, assets, virtualizer]);

  const busy = loading && assets.length === 0;

  return (
    <div ref={scrollRef} className={grid.scroll} data-testid="waterfall-view">
      {busy ? (
        <GridSpinner />
      ) : error ? (
        <GridError message={error} />
      ) : assets.length === 0 ? (
        <GridEmpty hint={emptyHint} />
      ) : (
        <div className={styles.sizer} style={{ height: virtualizer.getTotalSize() + PAD_Y * 2 }}>
          {virtualizer.getVirtualItems().map((item) => {
            const a = assets[item.index];
            if (!a) return null;
            const { aspectRatio } = waterfallItem(a.width, a.height, columnWidth);
            return (
              <div
                key={item.key}
                className={styles.item}
                style={{
                  width: columnWidth,
                  transform: `translate(${PAD_X + item.lane * (columnWidth + GAP)}px, ${item.start + PAD_Y}px)`,
                }}
              >
                <AssetCard
                  asset={a}
                  aspectRatio={aspectRatio}
                  selected={selection.isSelected(a.id)}
                  onSelect={(e) => selection.select(a.id, e)}
                  onToggleCheck={() => selection.toggle(a.id)}
                  onRate={(r) => onRate(a.id, r)}
                  onColor={(hex) => onColor(a.id, hex)}
                  onDetail={() => onDetail(a.id)}
                />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
