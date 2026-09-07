import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api/client";
import {
  type Asset,
  type BrowseLayout,
  isBrowseLayout,
  isLibraryView,
  type Tag,
  type ViewMode,
} from "./types";
import { useAssets } from "./hooks/useAssets";
import { useFacets } from "./hooks/useFacets";
import { useLibraryQuery } from "./hooks/useLibraryQuery";
import { useLibrary } from "./hooks/useLibrary";
import { useBoards } from "./hooks/useBoards";
import { useSelection } from "./hooks/useSelection";
import { useViewMode } from "./hooks/useViewMode";
import { Sidebar } from "./components/Sidebar";
import { SearchBar } from "./components/SearchBar";
import { AssetGrid } from "./components/AssetGrid";
import { WaterfallGrid } from "./components/WaterfallGrid";
import { GalleryView } from "./components/GalleryView";
import { ImmersiveViewer } from "./components/ImmersiveViewer";
import { AssetDetail } from "./components/AssetDetail";
import { BatchBar } from "./components/BatchBar";
import { TrashView } from "./components/TrashView";
import { InspectorPanel } from "./components/InspectorPanel";
import { BoardList } from "./components/BoardList";
import { BoardCanvas } from "./components/BoardCanvas";
import brand from "./components/Sidebar.module.css";
import styles from "./App.module.css";

export default function App() {
  const [view, setView] = useViewMode();
  // useLibraryQuery owns the composable filter/search query; filterFx bridges its
  // facet handlers to the composer's browse-restore + selection-clear, which
  // depend on view/selection defined below (issue #75).
  const filterFx = useRef<() => void>(() => {});
  const lq = useLibraryQuery(() => filterFx.current());
  const [version, setVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [inspectorTags, setInspectorTags] = useState<Tag[]>([]);
  const [activeBoardId, setActiveBoardId] = useState<string | null>(null);
  const [detail, setDetail] = useState<Asset | null>(null);
  const [immersiveIndex, setImmersiveIndex] = useState(0);
  // Keyboard cursor: tracks which card is "current" for Space-to-open-detail and
  // arrow-key navigation. Distinct from `sel.selected` (batch selection set).
  const [focusedId, setFocusedId] = useState<string | null>(null);

  // Ref populated by VideoPlayer / AudioPlayer when detail is open. App's Space
  // handler calls it so media toggles even when the player container lacks focus.
  const videoToggleRef = useRef<(() => void) | null>(null);

  const bump = useCallback(() => setVersion((v) => v + 1), []);
  const lib = useLibrary(setError);
  const boards = useBoards(setError);
  const { assets, loading, error: loadErr } = useAssets(view, lq.query, version);
  const kindCounts = useFacets(lq.query, version);
  const ids = useMemo(() => assets.map((a) => a.id), [assets]);
  const sel = useSelection(ids);

  // The inspector shows for a single selection; resolve it from the loaded list.
  const inspectedId = sel.count === 1 ? ([...sel.selected][0] ?? null) : null;
  const inspectedAsset = inspectedId
    ? assets.find((a) => a.id === inspectedId)
    : undefined;

  // Board views (list + canvas) own their full-area chrome, so the search and
  // batch bars hide there; every other view keeps the asset chrome.
  const isAssetView = view !== "boards" && view !== "board";

  // Remember the last browse layout so exiting immersive/trash/missing returns to it.
  const prevBrowse = useRef<BrowseLayout>(isBrowseLayout(view) ? view : "grid");
  useEffect(() => {
    if (isBrowseLayout(view)) prevBrowse.current = view;
  }, [view]);

  // Latest-value callback for facet changes: restore a browse layout (from a
  // special view) and clear the selection. Reassigned each render so it always
  // sees the current view/selection.
  filterFx.current = () => {
    setView(isBrowseLayout(view) ? view : prevBrowse.current);
    sel.clear();
  };

  useEffect(() => {
    if (error) {
      const t = setTimeout(() => setError(null), 4000);
      return () => clearTimeout(t);
    }
  }, [error]);

  useEffect(() => {
    if (notice) {
      const t = setTimeout(() => setNotice(null), 4000);
      return () => clearTimeout(t);
    }
  }, [notice]);

  // Load the inspected asset's tags; `version` refetches after batch mutations.
  useEffect(() => {
    if (!inspectedId) {
      setInspectorTags([]);
      return;
    }
    let alive = true;
    api
      .assetTags(inspectedId)
      .then((t) => {
        if (alive) setInspectorTags(t);
      })
      .catch(() => {
        if (alive) setInspectorTags([]);
      });
    return () => {
      alive = false;
    };
  }, [inspectedId, version]);

  // Wraps a batch call: run over the selection, clear it, then refresh assets
  // and the trash badge. `ids` overrides the selection for single-card actions.
  const run = useCallback(
    (fn: (targets: string[]) => Promise<unknown>, override?: string[]) => async () => {
      const targets = override ?? [...sel.selected];
      if (targets.length === 0) return;
      try {
        await fn(targets);
        if (!override) {
          sel.clear();
          setFocusedId(null);
        }
        bump();
        lib.refreshTrash();
        lib.refreshMissing();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [sel, bump, lib],
  );

  const changeView = (v: ViewMode) => {
    if (v === "immersive") {
      // Only derive the start index from the selection when the current dataset is
      // the library — a trash/missing selection index is meaningless once immersive
      // swaps to the library dataset.
      const firstSel = isLibraryView(view)
        ? assets.findIndex((a) => sel.selected.has(a.id))
        : -1;
      setImmersiveIndex(firstSel >= 0 ? firstSel : 0);
      setView("immersive");
      return;
    }
    // Trash/missing are distinct datasets — reset filters + selection. Switching
    // between grid/waterfall/gallery keeps the current library filter intact.
    if (v === "trash" || v === "missing") {
      sel.clear();
      lq.reset();
    }
    setView(v);
  };
  const openBoard = (id: string) => {
    sel.clear();
    setActiveBoardId(id);
    setView("board");
  };
  const createAndOpen = async () => {
    const created = await boards.createBoard("未命名图板");
    if (created) openBoard(created.id);
  };
  const setMissing = () => changeView("missing");

  // Send the current selection to a board. Unlike run(), this leaves the asset
  // grid untouched (the assets themselves don't change) — it clears the
  // selection and reports how many landed; `added` can be < the selection when
  // some assets were already on the board (skipped server-side).
  const sendToBoard = async (boardId: string, boardName: string) => {
    const targets = [...sel.selected];
    if (targets.length === 0) return;
    try {
      const { added } = await api.batchAddToBoard(boardId, targets);
      sel.clear();
      setNotice(`已添加 ${added} 张到「${boardName}」`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };
  // "新建图板…" from the batch bar: create a board (default name, matching the
  // board list) then send the selection to it.
  const sendToNewBoard = async () => {
    if (sel.count === 0) return;
    const created = await boards.createBoard("未命名图板");
    if (created) await sendToBoard(created.id, created.name);
  };

  // An active narrowing (search or a facet) means an empty grid is "no match",
  // not "empty library" — so the hint nudges toward relaxing the filter.
  const emptyHint =
    view === "trash"
      ? "回收站是空的。"
      : view === "missing"
        ? "没有丢失文件，所有索引文件均可访问。"
        : lq.hasFilter
          ? "没有匹配的素材，换个条件试试。"
          : "运行 `bin/hetu scan` 索引素材目录后即可浏览。";

  const rate = (id: string, rating: number) => void run((t) => api.rate(t, rating), [id])();
  const color = (id: string, hex: string) => void run((t) => api.colorLabel(t, hex), [id])();
  const openDetail = (id: string) => setDetail(assets.find((a) => a.id === id) ?? null);

  // App-level Space handler — single canonical path for Space across all views:
  //   • Immersive open            → skip (immersive owns its own keys)
  //   • Detail open (video/audio) → togglePlay via videoToggleRef
  //   • No detail + focused item  → open that item's detail panel
  //   • Input/textarea focused    → skip (let the field handle it)
  //
  // A ref bundle avoids stale-closure issues: the window listener is installed
  // once and reads live values at event time, same pattern as AssetGrid.
  const spaceCtxRef = useRef({ detail, focusedId, assets, view });
  spaceCtxRef.current = { detail, focusedId, assets, view };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== " ") return;
      const el = document.activeElement;
      if (
        el instanceof HTMLInputElement ||
        el instanceof HTMLTextAreaElement ||
        (el instanceof HTMLElement && el.isContentEditable)
      )
        return;
      const { detail: d, focusedId: fid, assets: list, view: v } = spaceCtxRef.current;
      // Immersive owns its own keys — never open a detail panel over it.
      if (v === "immersive") return;
      e.preventDefault();
      if (d && (d.kind === "video" || d.kind === "audio")) {
        videoToggleRef.current?.();
      } else if (!d && fid) {
        setDetail(list.find((a) => a.id === fid) ?? null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []); // empty deps: reads ref bundle at event time

  // The keyboard cursor is scoped to the current view + filter; drop it on either
  // change so Space/arrows never act on an item that has scrolled out of context.
  useEffect(() => {
    setFocusedId(null);
  }, [view, lq.query]);

  return (
    <div
      className={`app ${isAssetView && inspectedAsset && !detail ? "inspect" : ""}`}
      onClick={() => {
        sel.clear();
        setFocusedId(null);
      }}
    >
      <div className={brand.brand}>
        <span className={brand.logo}>河</span>
        <span className={brand.brandName}>
          hetu<small>DAM</small>
        </span>
      </div>

      <Sidebar
        folders={lib.folders}
        tags={lib.tags}
        activeFolder={lq.query.folderId}
        activeTag={lq.query.tagId}
        boardsActive={view === "boards" || view === "board"}
        onPickFolder={lq.setFolder}
        onPickTag={lq.setTag}
        onViewBoards={() => changeView("boards")}
        onCreateFolder={(n) => void lib.createFolder(n)}
        onDeleteFolder={(id) => void lib.deleteFolder(id)}
        onCreateTag={(n) => void lib.createTag(n)}
        onDeleteTag={(id) => void lib.deleteTag(id)}
        missingCount={lib.missingCount}
        onPickMissing={setMissing}
        activeMissing={view === "missing"}
        kindCounts={kindCounts}
        activeKinds={lq.query.kind}
        minRating={lq.query.minRating}
        onToggleKind={lq.toggleKind}
        onSetRating={lq.setRating}
        onClearFilters={lq.clearFilters}
      />

      {isAssetView && (
        <SearchBar
          view={view}
          trashCount={lib.trashCount}
          missingCount={lib.missingCount}
          onKeyword={lq.setKeyword}
          onColor={lq.setColor}
          onViewChange={changeView}
        />
      )}

      <div className={styles.main} onClick={(e) => e.stopPropagation()}>
        {view === "boards" ? (
          <BoardList
            boards={boards.list}
            onOpen={openBoard}
            onCreate={() => void createAndOpen()}
            onRename={(id, name) => void boards.renameBoard(id, name)}
            onDelete={(id) => void boards.deleteBoard(id)}
          />
        ) : view === "board" && activeBoardId ? (
          <BoardCanvas
            boardId={activeBoardId}
            tags={lib.tags}
            onBack={() => changeView("boards")}
            onError={setError}
          />
        ) : (
          <>
            {view === "trash" && (
              <TrashView count={lib.trashCount} onEmpty={() => void run(() => api.purgeTrash(0))()} />
            )}
            <div className={styles.gridWrap}>
              {view === "gallery" ? (
                <GalleryView
                  assets={assets}
                  loading={loading}
                  error={loadErr}
                  emptyHint={emptyHint}
                  selection={sel}
                  focusedId={focusedId}
                  onFocusChange={setFocusedId}
                  onDetail={openDetail}
                  onRate={rate}
                  onColor={color}
                />
              ) : view === "waterfall" ? (
                <WaterfallGrid
                  assets={assets}
                  loading={loading}
                  error={loadErr}
                  selection={sel}
                  focusedId={focusedId}
                  onFocusChange={setFocusedId}
                  emptyHint={emptyHint}
                  onRate={rate}
                  onColor={color}
                  onDetail={openDetail}
                />
              ) : (
                <AssetGrid
                  assets={assets}
                  loading={loading}
                  error={loadErr}
                  selection={sel}
                  focusedId={focusedId}
                  onFocusChange={setFocusedId}
                  emptyHint={emptyHint}
                  onRate={rate}
                  onColor={color}
                  onDetail={openDetail}
                />
              )}
            </div>
          </>
        )}
      </div>

      {isAssetView && inspectedAsset && !detail && (
        <InspectorPanel
          asset={inspectedAsset}
          tags={inspectorTags}
          onRate={(rating) => void run((t) => api.rate(t, rating), [inspectedAsset.id])()}
          onColor={(hex) => void run((t) => api.colorLabel(t, hex), [inspectedAsset.id])()}
          onNoteChange={(text) =>
            void run(() => api.updateNote(inspectedAsset.id, text), [inspectedAsset.id])()
          }
          onNoteDelete={() =>
            void run(() => api.deleteNote(inspectedAsset.id), [inspectedAsset.id])()
          }
        />
      )}

      {isAssetView && (
        <BatchBar
          count={sel.count}
          view={view}
          folders={lib.folders}
          tags={lib.tags}
          boards={boards.list}
          onClear={() => {
            sel.clear();
            setFocusedId(null);
          }}
          onTag={(tagId) => void run((t) => api.tag(t, [tagId]))()}
          onRate={(rating) => void run((t) => api.rate(t, rating))()}
          onColor={(hex) => void run((t) => api.colorLabel(t, hex))()}
          onMove={(folderId) => void run((t) => api.move(t, folderId))()}
          onAddToBoard={(boardId, boardName) => void sendToBoard(boardId, boardName)}
          onAddToNewBoard={() => void sendToNewBoard()}
          onTrash={() => void run((t) => api.trash(t))()}
          onRestore={() => void run((t) => api.restore(t))()}
        />
      )}

      {view === "immersive" && (
        <ImmersiveViewer
          assets={assets}
          startIndex={immersiveIndex}
          onExit={() => setView(prevBrowse.current)}
        />
      )}

      <AssetDetail
        asset={detail}
        toggleRef={videoToggleRef}
        onClose={() => {
          videoToggleRef.current = null;
          setDetail(null);
        }}
      />

      {error && <div className={styles.toast}>{error}</div>}
      {notice && <div className={`${styles.toast} ${styles.notice}`}>{notice}</div>}
    </div>
  );
}
