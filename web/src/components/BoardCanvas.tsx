import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Layer, Stage, Transformer } from "react-konva";
import type Konva from "konva";
import { type Asset, type AssetKind, type BoardItem, EMPTY_QUERY, type Query, type Tag } from "../types";
import { useAssets } from "../hooks/useAssets";
import { useFacets } from "../hooks/useFacets";
import { useBoard } from "../hooks/useBoards";
import { useBoardImages } from "../hooks/useBoardImages";
import { useCanvasViewport } from "../hooks/useCanvasViewport";
import { BoardAssetPanel } from "./BoardAssetPanel";
import { BoardCanvasItem } from "./BoardCanvasItem";
import { IconArrowLeft } from "./icons";
import styles from "./BoardCanvas.module.css";

interface Props {
  boardId: string;
  tags: Tag[];
  onBack: () => void;
  onError: (msg: string) => void;
}

// A dropped item's initial box: scale the asset's natural size into DROP_MAX,
// falling back to a sensible box when dimensions are unknown.
const DROP_MAX = 240;
const FALLBACK = { w: 200, h: 150 };

function dropSize(asset: Asset | undefined): { w: number; h: number } {
  if (!asset || asset.width <= 0 || asset.height <= 0) return FALLBACK;
  const s = Math.min(1, DROP_MAX / Math.max(asset.width, asset.height));
  return { w: Math.round(asset.width * s), h: Math.round(asset.height * s) };
}

// The infinite-canvas editor (ViewMode "board"): a drag-source panel on the
// left and a Konva stage on the right. Pan/zoom come from useCanvasViewport,
// per-item drag/resize from BoardCanvasItem, and persistence from useBoard.
export function BoardCanvas({ boardId, tags, onBack, onError }: Props) {
  const { board, items, addItem, updateItems, removeItem } = useBoard(boardId, onError);
  const { scale, pos, panning, onWheel, onStageDragEnd } = useCanvasViewport();

  // The drag-source panel reuses the main library's query model + hooks: search
  // and tag/format/star facets narrow it server-side (issue #75).
  const [boardQuery, setBoardQuery] = useState<Query>(EMPTY_QUERY);
  const { assets, loading: loadingAssets, error: assetErr } = useAssets("grid", boardQuery, 0);
  const kindCounts = useFacets(boardQuery, 0);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [size, setSize] = useState({ w: 0, h: 0 });

  const wrapRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<Konva.Stage>(null);
  const trRef = useRef<Konva.Transformer>(null);

  const assetIds = useMemo(() => items.map((it) => it.asset_id), [items]);
  const images = useBoardImages(assetIds);
  const sorted = useMemo(() => [...items].sort((a, b) => a.z - b.z), [items]);

  // Surface panel asset-load failures through the board's error channel.
  useEffect(() => {
    if (assetErr) onError(assetErr);
  }, [assetErr, onError]);

  // Board-local facet handlers: search + tag/format/star narrow the panel via
  // the same query model as the main library (tag toggles off on re-pick; the
  // star facet clears via RatingStars re-click).
  const setKeyword = useCallback((keyword: string) => setBoardQuery((q) => ({ ...q, keyword })), []);
  const pickTag = useCallback(
    (tagId: string) => setBoardQuery((q) => ({ ...q, tagId: q.tagId === tagId ? null : tagId })),
    [],
  );
  const toggleKind = useCallback(
    (kind: AssetKind) =>
      setBoardQuery((q) => ({
        ...q,
        kind: q.kind.includes(kind) ? q.kind.filter((k) => k !== kind) : [...q.kind, kind],
      })),
    [],
  );
  const setRating = useCallback((rating: number) => setBoardQuery((q) => ({ ...q, minRating: rating })), []);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const measure = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Keep the Transformer bound to the selected node as items re-render.
  useEffect(() => {
    const tr = trRef.current;
    const stage = stageRef.current;
    if (!tr || !stage) return;
    const node = selectedId ? stage.findOne(`#${selectedId}`) : undefined;
    tr.nodes(node ? [node] : []);
    tr.getLayer()?.batchDraw();
  }, [selectedId, items]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Delete" && e.key !== "Backspace") return;
      if (!selectedId) return;
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;
      void removeItem(selectedId);
      setSelectedId(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selectedId, removeItem]);

  const patchItem = useCallback(
    (id: string, patch: Partial<BoardItem>) =>
      updateItems(items.map((it) => (it.id === id ? { ...it, ...patch } : it))),
    [items, updateItems],
  );

  const selectItem = useCallback(
    (id: string) => {
      setSelectedId(id);
      const maxZ = items.reduce((m, it) => Math.max(m, it.z), 0);
      const target = items.find((it) => it.id === id);
      if (target && target.z < maxZ) patchItem(id, { z: maxZ + 1 });
    },
    [items, patchItem],
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

  const onStageMouseDown = useCallback((e: Konva.KonvaEventObject<MouseEvent>) => {
    if (e.target === e.target.getStage()) setSelectedId(null);
  }, []);

  return (
    <div className={styles.view}>
      <div className={styles.toolbar}>
        <button className="btn btn-ghost" onClick={onBack}>
          <IconArrowLeft width={15} height={15} /> 返回图板列表
        </button>
        <span className={styles.name}>{board?.name ?? "…"}</span>
        <div className={styles.spacer} />
        <span className={styles.hint}>滚轮缩放 · 空格/中键拖拽平移 · Delete 删除</span>
      </div>

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
                  image={images.get(it.asset_id)}
                  selected={it.id === selectedId}
                  onSelect={() => selectItem(it.id)}
                  onChange={(patch) => patchItem(it.id, patch)}
                />
              ))}
              <Transformer ref={trRef} rotateEnabled flipEnabled={false} />
            </Layer>
          </Stage>
        </div>
      </div>
    </div>
  );
}
