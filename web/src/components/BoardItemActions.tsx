import type { BoardItem } from "../types";
import type { BoardPickers } from "../hooks/useBoardPickers";
import { VideoFramePicker } from "./VideoFramePicker";
import { ModelAnglePicker } from "./ModelAnglePicker";

interface Props {
  selectedItems: BoardItem[];
  pickers: BoardPickers;
}

// The per-item frame/angle actions for the board (#86): a "选取帧"/"选取角度"
// button shown when exactly one video/model item is selected, plus the picker
// modals. Picker state lives in the parent (BoardCanvas) so a double-click on
// the canvas item opens the very same modal — this component only renders the
// toolbar buttons and the modals. The item kind comes from the item's own
// asset_kind (resolved server-side in the board response), so the button shows
// even when the asset panel's query has not loaded that asset.
export function BoardItemActions({ selectedItems, pickers }: Props) {
  const sole = selectedItems.length === 1 ? selectedItems[0] : undefined;
  const soleKind = sole && sole.kind !== "note" ? sole.asset_kind : undefined;
  const { frame, angle } = pickers;

  return (
    <>
      {sole && soleKind === "video" && (
        <button
          className="btn btn-ghost"
          data-testid="pick-frame-btn"
          onClick={() => pickers.open(sole)}
        >
          选取帧
        </button>
      )}
      {sole && soleKind === "model" && (
        <button
          className="btn btn-ghost"
          data-testid="pick-angle-btn"
          onClick={() => pickers.open(sole)}
        >
          选取角度
        </button>
      )}
      {frame && (
        <VideoFramePicker
          asset={frame.asset}
          {...(frame.frameMs != null ? { initialMs: frame.frameMs } : {})}
          onCapture={pickers.onFrameCapture}
          onClose={pickers.close}
        />
      )}
      {angle && (
        <ModelAnglePicker
          asset={angle.asset}
          {...(angle.view ? { initialView: angle.view } : {})}
          onCapture={pickers.onAngleCapture}
          onClose={pickers.close}
        />
      )}
    </>
  );
}
