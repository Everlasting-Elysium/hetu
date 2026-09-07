import type { Asset, BoardItem } from "../types";
import { useBoardPickers } from "../hooks/useBoardPickers";
import type { BoardCaptures } from "../hooks/useBoardCaptures";
import { VideoFramePicker } from "./VideoFramePicker";
import { ModelAnglePicker } from "./ModelAnglePicker";

interface Props {
  selectedItems: BoardItem[];
  assetById: Map<string, Asset>;
  patchItem: (id: string, patch: Partial<BoardItem>) => void;
  captures: BoardCaptures;
}

// The per-item frame/angle actions for the board (#86): a "选取帧"/"选取角度" button
// shown when exactly one video/model item is selected, plus the picker modal it
// opens. The two live together so the whole feature is one unit; the button
// flows into the toolbar while the modal (position:fixed) overlays the viewport,
// so a single fragment mounted in the toolbar renders both correctly.
export function BoardItemActions({ selectedItems, assetById, patchItem, captures }: Props) {
  const pickers = useBoardPickers(patchItem, captures);
  const sole = selectedItems.length === 1 ? selectedItems[0] : undefined;
  const asset = sole && sole.kind !== "note" ? assetById.get(sole.asset_id) : undefined;
  const { frame, angle } = pickers;

  return (
    <>
      {asset?.kind === "video" && sole && (
        <button
          className="btn btn-ghost"
          data-testid="pick-frame-btn"
          onClick={() => pickers.open(sole, asset)}
        >
          选取帧
        </button>
      )}
      {asset?.kind === "model" && sole && (
        <button
          className="btn btn-ghost"
          data-testid="pick-angle-btn"
          onClick={() => pickers.open(sole, asset)}
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
