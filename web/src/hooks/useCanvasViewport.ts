import { useCallback, useEffect, useState } from "react";
import Konva from "konva";

type WheelEvt = Konva.KonvaEventObject<WheelEvent>;
type DragEvt = Konva.KonvaEventObject<DragEvent>;

const MIN_SCALE = 0.15;
const MAX_SCALE = 5;
const ZOOM_STEP = 1.05;

export interface Viewport {
  scale: number;
  pos: { x: number; y: number };
  panning: boolean;
  onWheel: (e: WheelEvt) => void;
  onStageDragEnd: (e: DragEvt) => void;
}

// Infinite-canvas viewport state driven entirely by Konva's built-in stage
// drag + wheel. Wheel zooms toward the cursor; the stage itself is draggable
// (left or middle button) to pan. `panning` (space held) only styles the cursor.
export function useCanvasViewport(): Viewport {
  const [scale, setScale] = useState(1);
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const [panning, setPanning] = useState(false);

  useEffect(() => {
    Konva.dragButtons = [0, 1];
  }, []);

  useEffect(() => {
    const onKey = (down: boolean) => (e: KeyboardEvent) => {
      if (e.code === "Space") setPanning(down);
    };
    const down = onKey(true);
    const up = onKey(false);
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
    };
  }, []);

  const onWheel = useCallback((e: WheelEvt) => {
    e.evt.preventDefault();
    const stage = e.target.getStage();
    const pointer = stage?.getPointerPosition();
    if (!stage || !pointer) return;
    const old = stage.scaleX();
    const world = {
      x: (pointer.x - stage.x()) / old,
      y: (pointer.y - stage.y()) / old,
    };
    const raw = e.evt.deltaY > 0 ? old / ZOOM_STEP : old * ZOOM_STEP;
    const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, raw));
    setScale(next);
    setPos({ x: pointer.x - world.x * next, y: pointer.y - world.y * next });
  }, []);

  const onStageDragEnd = useCallback((e: DragEvt) => {
    if (e.target === e.target.getStage()) {
      setPos({ x: e.target.x(), y: e.target.y() });
    }
  }, []);

  return { scale, pos, panning, onWheel, onStageDragEnd };
}
