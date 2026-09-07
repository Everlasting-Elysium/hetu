import { useCallback, useState } from "react";
import type { Asset, BoardItem } from "../types";
import type { BoardCaptures } from "./useBoardCaptures";

// The item + asset a picker modal is editing. `asset` supplies the file/model
// URL and kind the picker needs; `itemId` is where the capture is folded back;
// frameMs/view are the item's current pin, captured at open time so the modal
// seeds itself without re-reading the live item list.
export interface PickerTarget {
  itemId: string;
  asset: Asset;
  frameMs: number | null;
  view: string;
}

export interface BoardPickers {
  frame: PickerTarget | null;
  angle: PickerTarget | null;
  open: (item: BoardItem, asset: Asset) => void;
  close: () => void;
  onFrameCapture: (blob: Blob, ms: number) => void;
  onAngleCapture: (blob: Blob, view: string) => void;
}

// Owns the video-frame / model-angle picker modals for the board canvas (#86):
// which item is being edited, routed by asset kind, and folding a capture back
// onto that item — the captured still becomes the item's image override (via
// captures) and the frame ms / camera pose is patched onto the item so it
// persists. Kept out of BoardCanvas so the canvas stays a thin composer.
export function useBoardPickers(
  patchItem: (id: string, patch: Partial<BoardItem>) => void,
  captures: BoardCaptures,
): BoardPickers {
  const [frame, setFrame] = useState<PickerTarget | null>(null);
  const [angle, setAngle] = useState<PickerTarget | null>(null);

  const open = useCallback((item: BoardItem, asset: Asset) => {
    const target: PickerTarget = {
      itemId: item.id,
      asset,
      frameMs: item.frame_ms ?? null,
      view: item.view ?? "",
    };
    if (asset.kind === "video") setFrame(target);
    else if (asset.kind === "model") setAngle(target);
  }, []);

  const close = useCallback(() => {
    setFrame(null);
    setAngle(null);
  }, []);

  const onFrameCapture = useCallback(
    (blob: Blob, ms: number) => {
      if (!frame) return;
      captures.set(frame.itemId, blob);
      patchItem(frame.itemId, { frame_ms: Math.round(ms) });
      setFrame(null);
    },
    [frame, captures, patchItem],
  );

  const onAngleCapture = useCallback(
    (blob: Blob, view: string) => {
      if (!angle) return;
      captures.set(angle.itemId, blob);
      patchItem(angle.itemId, { view });
      setAngle(null);
    },
    [angle, captures, patchItem],
  );

  return { frame, angle, open, close, onFrameCapture, onAngleCapture };
}
