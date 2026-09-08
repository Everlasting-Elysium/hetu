import type { BoardItem } from "../types";
import type { BoardPickers } from "../hooks/useBoardPickers";
import { BoardItemActions } from "./BoardItemActions";
import { IconArrowLeft, IconCompress, IconExpand, IconPlus } from "./icons";
import styles from "./BoardCanvas.module.css";

interface Props {
  boardName: string;
  isFullscreen: boolean;
  selectedItems: BoardItem[];
  pickers: BoardPickers;
  onBack: () => void;
  onToggleFullscreen: () => void;
  onAddNote: () => void;
  onExport: () => void;
  onDuplicate: () => void;
}

// The top bar of the board canvas: back button, board name, fullscreen toggle,
// add-note shortcut, export button, duplicate button, and per-item actions
// (frame/angle pickers). Picker state is owned by BoardCanvas and passed
// through so a canvas double-click and the toolbar button drive the same modal.
export function BoardToolbar({
  boardName,
  isFullscreen,
  selectedItems,
  pickers,
  onBack,
  onToggleFullscreen,
  onAddNote,
  onExport,
  onDuplicate,
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
      {selectedItems.length > 0 && (
        <button
          className="btn btn-ghost"
          data-testid="duplicate-btn"
          title="复制选中项 (⌘D)"
          onClick={onDuplicate}
        >
          复制 ({selectedItems.length})
        </button>
      )}
      <BoardItemActions selectedItems={selectedItems} pickers={pickers} />
      <div className={styles.spacer} />
      <span className={styles.hint}>
        滚轮缩放 · 空格/中键平移 · 框选多选 · ⌘D 复制 · Delete 删除
      </span>
    </div>
  );
}
