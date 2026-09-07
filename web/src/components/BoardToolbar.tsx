import type { Asset, BoardItem } from "../types";
import type { BoardCaptures } from "../hooks/useBoardCaptures";
import { BoardItemActions } from "./BoardItemActions";
import { IconArrowLeft, IconCompress, IconExpand, IconPlus } from "./icons";
import styles from "./BoardCanvas.module.css";

interface Props {
  boardName: string;
  isFullscreen: boolean;
  selectedItems: BoardItem[];
  assetById: Map<string, Asset>;
  patchItem: (id: string, patch: Partial<BoardItem>) => void;
  captures: BoardCaptures;
  onBack: () => void;
  onToggleFullscreen: () => void;
  onAddNote: () => void;
  onExport: () => void;
}

// The top bar of the board canvas: back button, board name, fullscreen toggle,
// add-note shortcut, export button, and per-item actions (frame/angle pickers).
export function BoardToolbar({
  boardName,
  isFullscreen,
  selectedItems,
  assetById,
  patchItem,
  captures,
  onBack,
  onToggleFullscreen,
  onAddNote,
  onExport,
}: Props) {
  return (
    <div className={styles.toolbar}>
      <button className="btn btn-ghost" onClick={onBack}>
        <IconArrowLeft width={15} height={15} /> 返回图板列表
      </button>
      <span className={styles.name}>{boardName}</span>
      <button
        className="btn btn-ghost btn-icon"
        data-testid="fullscreen-btn"
        title={isFullscreen ? "退出全屏" : "全屏"}
        onClick={onToggleFullscreen}
      >
        {isFullscreen ? <IconCompress width={16} height={16} /> : <IconExpand width={16} height={16} />}
      </button>
      <button className="btn btn-ghost" onClick={onAddNote}>
        <IconPlus width={15} height={15} /> 便签
      </button>
      <button className="btn btn-ghost" data-testid="export-btn" onClick={onExport}>
        导出
      </button>
      <BoardItemActions
        selectedItems={selectedItems}
        assetById={assetById}
        patchItem={patchItem}
        captures={captures}
      />
      <div className={styles.spacer} />
      <span className={styles.hint}>滚轮缩放 · 空格/中键拖拽平移 · Delete 删除</span>
    </div>
  );
}
