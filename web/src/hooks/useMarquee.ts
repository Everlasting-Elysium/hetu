import { useCallback, useEffect, useRef, useState } from "react";
import type Konva from "konva";
import type { BoardItem } from "../types";

export interface MarqueeRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface UseMarquee {
  rect: MarqueeRect | null;
  onStageMouseDown: (e: Konva.KonvaEventObject<MouseEvent>) => void;
}

// Minimum drag distance (world units) to distinguish a click from a marquee.
const MIN_DRAG = 4;

function intersects(item: BoardItem, r: MarqueeRect): boolean {
  return (
    item.x + item.w > r.x &&
    item.x < r.x + r.width &&
    item.y + item.h > r.y &&
    item.y < r.y + r.height
  );
}

// Rubber-band selection: left-click drag on the stage background draws a
// rectangle; on release every item whose bounding box overlaps is selected via
// onSelect. Pan (space held or middle-button) is unaffected — the caller uses
// dragBoundFunc to lock the Stage position during marquee so the two never
// fight, while Konva.dragButtons stays at [0,1] so items remain left-draggable.
export function useMarquee(
  items: BoardItem[],
  panning: boolean,
  stageRef: React.RefObject<Konva.Stage | null>,
  onSelect: (ids: string[]) => void,
): UseMarquee {
  const [rect, setRect] = useState<MarqueeRect | null>(null);
  const origin = useRef<{ x: number; y: number } | null>(null);
  const itemsRef = useRef(items);
  const onSelectRef = useRef(onSelect);
  itemsRef.current = items;
  onSelectRef.current = onSelect;

  const screenToWorld = useCallback(
    (clientX: number, clientY: number): { x: number; y: number } | null => {
      const stage = stageRef.current;
      const container = stage?.container();
      if (!stage || !container) return null;
      const bounds = container.getBoundingClientRect();
      const sx = clientX - bounds.left;
      const sy = clientY - bounds.top;
      return {
        x: (sx - stage.x()) / stage.scaleX(),
        y: (sy - stage.y()) / stage.scaleY(),
      };
    },
    [stageRef],
  );

  // Window-level move/up so the marquee tracks reliably even when the cursor
  // leaves the canvas during a fast drag.
  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!origin.current) return;
      const pos = screenToWorld(e.clientX, e.clientY);
      if (!pos) return;
      const o = origin.current;
      setRect({
        x: Math.min(o.x, pos.x),
        y: Math.min(o.y, pos.y),
        width: Math.abs(pos.x - o.x),
        height: Math.abs(pos.y - o.y),
      });
    };

    const onUp = () => {
      setRect((r) => {
        if (origin.current && r && (r.width > MIN_DRAG || r.height > MIN_DRAG)) {
          const hit = itemsRef.current
            .filter((it) => intersects(it, r))
            .map((it) => it.id);
          if (hit.length > 0) onSelectRef.current(hit);
        }
        origin.current = null;
        return null;
      });
    };

    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
  }, [screenToWorld]);

  const onStageMouseDown = useCallback(
    (e: Konva.KonvaEventObject<MouseEvent>) => {
      if (e.evt.button !== 0 || panning) return;
      if (e.target !== e.target.getStage()) return;
      const pos = screenToWorld(e.evt.clientX, e.evt.clientY);
      if (!pos) return;
      origin.current = pos;
      setRect({ x: pos.x, y: pos.y, width: 0, height: 0 });
    },
    [panning, screenToWorld],
  );

  return { rect, onStageMouseDown };
}
