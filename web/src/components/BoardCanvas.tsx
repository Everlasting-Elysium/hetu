import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Layer, Stage, Transformer } from "react-konva";
import type Konva from "konva";
import type { Asset, BoardItem, Tag } from "../types";
import { useAssets } from "../hooks/useAssets";
import { useFacets } from "../hooks/useFacets";
import { useBoard } from "../hooks/useBoards";
import { useBoardImages } from "../hooks/useBoardImages";
import { useBoardCaptures } from "../hooks/useBoardCaptures";
import { useBoardPickers } from "../hooks/useBoardPickers";
import { useBoardPanelQuery } from "../hooks/useBoardPanelQuery";
import { useBoardSelection } from "../hooks/useBoardSelection";
import { useArrangeShortcuts } from "../hooks/useArrangeShortcuts";
import { useCanvasViewport } from "../hooks/useCanvasViewport";
import { useFullscreen } from "../hooks/useFullscreen";
import { BoardAssetPanel } from "./BoardAssetPanel";
import { BoardCanvasItem } from "./BoardCanvasItem";
import { BoardAlignToolbar } from "./BoardAlignToolbar";
import { BoardExportDialog } from "./BoardExportDialog";
import { BoardNoteEditor } from "./BoardNoteEditor";
import { BoardToolbar } from "./BoardToolbar";
import { applyAlign as computeAlign } from "./boardAlign";
import { boardContentRect, exportBoard } from "./boardExport";
import styles from "./BoardCanvas.module.css";

interface Props {
  boardId: string;
  tags: Tag[];
  onBack: () => void;
  onError: (msg: string) => void;
}

// A dropped item's initial box: scale the asset's natural size into DROP_MAX,
// falling back to a sensible box when dimensions are unknown. Notes drop at a
// fixed box since they have no intrinsic size.
const DROP_MAX = 240;
const FALLBACK = { w: 200, h: 150 };
const NOTE_BOX = { w: 200, h: 150 };

function dropSize(asset: Asset | undefined): { w: number; h: number } {
  if (!asset || asset.width <= 0 || asset.height <= 0) return FALLBACK;
  const s = Math.min(1, DROP_MAX / Math.max(asset.width, asset.height));
  return { w: Math.round(asset.width * s), h: Math.round(asset.height * s) };
}

// The infinite-canvas editor (ViewMode "board"): a drag-source panel on the
// left and a Konva stage on the right. Pan/zoom come from useCanvasViewport,
// multi-selection + Transformer from useBoardSelection, per-item drag/resize
// from BoardCanvasItem, and persistence from useBoard. Fullscreen, the arrange
// toolbar + keyboard shortcuts, note editing, frame/angle pickers, and PNG
// export layer on top.
export function BoardCanvas({ boardId, tags, onBack, onError }: Props) {
  const { board, items, addItem, addNote, updateItems, patchItem, removeItem } = useBoard(boardId, onError);
  const { scale, pos, panning, onWheel, onStageDragEnd } = useCanvasViewport();
  const { boardQuery, setKeyword, pickTag, toggleKind, setRating } = useBoardPanelQuery();
  const { assets, loading: loadingAssets, error: assetErr } = useAssets("grid", boardQuery, 0);
  const kindCounts = useFacets(boardQuery, 0);

  const [size, setSize] = useState({ w: 0, h: 0 });
  const [editingId, setEditingId] = useState<string | null>(null);
  const [exportOpen, setExportOpen] = useState(false);

  const viewRef = useRef<HTMLDivElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<Konva.Stage>(null);
  const trRef = useRef<Konva.Transformer>(null);

  const { isFullscreen, toggle: toggleFullscreen } = useFullscreen(viewRef);

  // Per-item captured stills (frame/angle picker) override an item's default
  // thumbnail; useBoardImages resolves the rest (pinned frame -> frameUrl, else
  // the asset thumb) and skips notes internally. Picker state is owned here so a
  // canvas double-click and the toolbar button drive the same modal.
  const captures = useBoardCaptures();
  const pickers = useBoardPickers(patchItem, captures);
  const images = useBoardImages(items, captures.urls);
  const sorted = useMemo(() => [...items].sort((a, b) => a.z - b.z), [items]);

  useEffect(() => {
    if (assetErr) onError(assetErr);
  }, [assetErr, onError]);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const measure = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const { selectedIds, selectItem, clear } = useBoardSelection({
    items,
    patchItem,
    removeItem,
    stageRef,
    trRef,
  });
  const selectedItems = useMemo(
    () => items.filter((it) => selectedIds.has(it.id)),
    [items, selectedIds],
  );

  // Merge an arrange result's new geometry back over the full item list, so it
  // flows through the same debounced batch PATCH as drag/resize.
  const applyArrange = useCallback(
    (updated: BoardItem[]) => {
      const byId = new Map(updated.map((u) => [u.id, u]));
      updateItems(items.map((it) => byId.get(it.id) ?? it));
    },
    [items, updateItems],
  );

  // PureRef-style keyboard arranging (Ctrl/Cmd+Arrow align, +Alt normalize size,
  // +Alt+Shift distribute), active only with two or more items selected.
  useArrangeShortcuts({
    selectedItems,
    onApply: (op) => applyArrange(computeAlign(op, selectedItems)),
  });

  // Edit routing: notes open the inline text editor; video/model items open
  // their frame/angle picker. Kind comes from item.asset_kind (resolved
  // server-side), so it works regardless of the asset panel's current query.
  const editItem = useCallback(
    (item: BoardItem) => {
      if (item.kind === "note") setEditingId(item.id);
      else pickers.open(item);
    },
    [pickers],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent<HTMLDivElement>) => {
      e.preventDefault();
      const stage = stageRef.current;
      if (!stage) return;
      const assetId = e.dataTransfer.getData("text/plain");
      if (!assetId) return;
      stage.setPointersPositions(e.nativeEvent);
      const p = stage.getPointerPosition();
      if (!p) return;
      const wx = (p.x - stage.x()) / stage.scaleX();
      const wy = (p.y - stage.y()) / stage.scaleY();
      const { w, h } = dropSize(assets.find((a) => a.id === assetId));
      void addItem(assetId, { x: wx - w / 2, y: wy - h / 2, w, h });
    },
    [assets, addItem],
  );

  // Drop a note at the current viewport center (world coords) and open it for
  // editing straight away.
  const addNoteAtCenter = useCallback(async () => {
    const cx = (size.w / 2 - pos.x) / scale;
    const cy = (size.h / 2 - pos.y) / scale;
    const created = await addNote("New Note", {
      x: cx - NOTE_BOX.w / 2,
      y: cy - NOTE_BOX.h / 2,
      ...NOTE_BOX,
    });
    if (created) setEditingId(created.id);
  }, [size, pos, scale, addNote]);

  const doExport = useCallback((pixelRatio: number) => {
    const stage = stageRef.current;
    if (stage) exportBoard(stage, pixelRatio);
    setExportOpen(false);
  }, []);

  const onStageMouseDown = useCallback(
    (e: Konva.KonvaEventObject<MouseEvent>) => {
      if (e.target === e.target.getStage()) clear();
    },
    [clear],
  );

  const editing = editingId ? items.find((it) => it.id === editingId) : undefined;

  return (
    <div ref={viewRef} className={styles.view}>
      <BoardToolbar
        boardName={board?.name ?? "…"}
        isFullscreen={isFullscreen}
        selectedItems={selectedItems}
        pickers={pickers}
        onBack={onBack}
        onToggleFullscreen={toggleFullscreen}
        onAddNote={addNoteAtCenter}
        onExport={() => setExportOpen(true)}
      />

      {selectedItems.length >= 2 && (
        <BoardAlignToolbar selectedItems={selectedItems} onUpdate={applyArrange} />
      )}

      <div className={styles.body}>
        <BoardAssetPanel
          assets={assets}
          loading={loadingAssets}
          tags={tags}
          query={boardQuery}
          kindCounts={kindCounts}
          onKeyword={setKeyword}
          onPickTag={pickTag}
          onToggleKind={toggleKind}
          onSetRating={setRating}
        />
        <div
          ref={wrapRef}
          className={`${styles.canvas} ${panning ? styles.panning : ""}`}
          onDrop={handleDrop}
          onDragOver={(e) => e.preventDefault()}
        >
          <Stage
            ref={stageRef}
            width={size.w}
            height={size.h}
            scaleX={scale}
            scaleY={scale}
            x={pos.x}
            y={pos.y}
            draggable
            onWheel={onWheel}
            onDragEnd={onStageDragEnd}
            onMouseDown={onStageMouseDown}
          >
            <Layer>
              {sorted.map((it) => (
                <BoardCanvasItem
                  key={it.id}
                  item={it}
                  image={images.get(it.id)}
                  selected={selectedIds.has(it.id)}
                  onSelect={(mods) => selectItem(it.id, mods)}
                  onEdit={() => editItem(it)}
                  onChange={(patch) => patchItem(it.id, patch)}
                />
              ))}
              <Transformer ref={trRef} rotateEnabled flipEnabled={false} />
            </Layer>
          </Stage>
          {editing && (
            <BoardNoteEditor
              item={editing}
              scale={scale}
              pos={pos}
              onCommit={(text) => {
                if (text.trim() === "") {
                  void removeItem(editing.id);
                } else {
                  patchItem(editing.id, { text });
                }
                setEditingId(null);
              }}
              onCancel={() => setEditingId(null)}
            />
          )}
        </div>
      </div>

      {exportOpen && (
        <BoardExportDialog
          contentSize={boardContentRect(stageRef.current)}
          onExport={doExport}
          onClose={() => setExportOpen(false)}
        />
      )}
    </div>
  );
}
