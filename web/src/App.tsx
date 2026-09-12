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
import { useBoardPanelQuery } from "./hooks/useBoardPanelQuery";
import { useLibrary } from "./hooks/useLibrary";
import { useBoards } from "./hooks/useBoards";
import { useCollections } from "./hooks/useCollections";
import { useSelection } from "./hooks/useSelection";
import { useViewMode } from "./hooks/useViewMode";
import { useImport } from "./hooks/useImport";
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
import { CollectionView } from "./components/CollectionView";
import { copyAssetsToClipboard } from "./lib/clipboard";
import brand from "./components/Sidebar.module.css";
import styles from "./App.module.css";

export default function App() {
  const [view, setView] = useViewMode();
  // useLibraryQuery owns the composable filter/search query; filterFx bridges its
  // facet handlers to the composer's browse-restore + selection-clear, which
  // depend on view/selection defined below (issue #75).
  const filterFx = useRef<() => void>(() => {});
  const lq = useLibraryQuery(() => filterFx.current());
  // The board canvas's drag-source panel (BoardAssetPanel) is scoped by this
  // board-local query rather than `lq`. Lifted here (issue #108) so the global
  // sidebar can drive it: BoardCanvas used to own this internally, but the
  // sidebar is the one place a user picks folder/tag/format/star, and it needs
  // to target whichever dataset is on screen.
  const bpq = useBoardPanelQuery();
  const [version, setVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [inspectorTags, setInspectorTags] = useState<Tag[]>([]);
  const [activeBoardId, setActiveBoardId] = useState<string | null>(null);
  const [activeCollectionId, setActiveCollectionId] = useState<string | null>(null);
  // Bumped when an asset is dropped onto the currently-open collection's sidebar
  // node, so CollectionView refetches its member grid without a remount.
  const [colRefresh, setColRefresh] = useState(0);
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
  const collections = useCollections(setError);
  const { assets, loading, error: loadErr } = useAssets(view, lq.query, version);
  // The sidebar's facet controls act on whichever query is "active" for the
  // current view: the board panel's query on the board canvas, `lq` everywhere
  // else (issue #108). This single switch is the entire routing — the sidebar
  // component itself stays oblivious to which query backs it.
  const activeQuery = view === "board" ? bpq.boardQuery : lq.query;
  const kindCounts = useFacets(activeQuery, version);
  const ids = useMemo(() => assets.map((a) => a.id), [assets]);
  const sel = useSelection(ids);

  // Paste (Ctrl/⌘+V) + external file drag-drop import for the asset page (#89).
  // Enabled only on the browse layouts; imports land in the active folder when
  // one is selected. Both entry points funnel through the hook's importFiles.
  const { dragging, dropHandlers } = useImport({
    folderId: lq.query.folderId,
    enabled: isBrowseLayout(view),
    onDone: bump,
    onNotice: setNotice,
    onError: setError,
  });

  // The inspector shows for a single selection; resolve it from the loaded list.
  const inspectedId = sel.count === 1 ? ([...sel.selected][0] ?? null) : null;
  const inspectedAsset = inspectedId
    ? assets.find((a) => a.id === inspectedId)
    : undefined;

  // Board + collection detail views own their full-area chrome, so the search and
  // batch bars hide there; every other view keeps the asset chrome.
  const isAssetView = view !== "boards" && view !== "board" && view !== "collection";

  // The active collection (resolved from the tree list) — null while none is open
  // or after it (or an ancestor) was deleted, which the effect below navigates on.
  const activeCollection =
    collections.list.find((c) => c.id === activeCollectionId) ?? null;

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
  const openCollection = (id: string) => {
    sel.clear();
    setActiveCollectionId(id);
    setView("collection");
  };
  // Drop an asset (dragged from any grid card) onto a sidebar collection node.
  // Reuses the notice/error toasts; refetches the member grid if that collection
  // is the one currently open.
  const dropOnCollection = async (collectionId: string, assetId: string) => {
    if (!(await collections.addItem(collectionId, assetId))) return;
    const c = collections.list.find((x) => x.id === collectionId);
    setNotice(`已加入「${c?.name ?? "合集"}」`);
    if (collectionId === activeCollectionId) setColRefresh((v) => v + 1);
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
  const favorite = (id: string, fav: boolean) => void run((t) => api.favorite(t, fav), [id])();
  const openDetail = (id: string) => setDetail(assets.find((a) => a.id === id) ?? null);

  // App-level keyboard handler — single canonical path for shortcuts across all
  // views. Currently handles Space (detail/play) and Ctrl/Cmd+C (clipboard copy).
  //
  // A ref bundle avoids stale-closure issues: the window listener is installed
  // once and reads live values at event time, same pattern as AssetGrid.
  const keyCtxRef = useRef({ detail, focusedId, assets, view, selected: sel.selected });
  keyCtxRef.current = { detail, focusedId, assets, view, selected: sel.selected };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = document.activeElement;
      const isInput =
        el instanceof HTMLInputElement ||
        el instanceof HTMLTextAreaElement ||
        (el instanceof HTMLElement && el.isContentEditable);

      // --- Ctrl/Cmd+C: copy selected assets to system clipboard (#90) ---
      // toLowerCase so CapsLock-on (e.key === "C") still triggers; the !shiftKey
      // guard below still excludes Shift+Cmd+C.
      if (e.key.toLowerCase() === "c" && (e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey) {
        // Never hijack native text copy: input fields, or user has text selected.
        if (isInput) return;
        const textSel = window.getSelection();
        if (textSel && textSel.toString().length > 0) return;

        const { assets: list, selected: s, focusedId: fid, view: v } = keyCtxRef.current;
        // Immersive owns its own Ctrl+C — see ImmersiveViewer.
        if (v === "immersive") return;
        // No selection and no focus → nothing to copy, let browser handle it.
        if (s.size === 0 && !fid) return;

        e.preventDefault();
        copyAssetsToClipboard(list, s, fid)
          .then((result) => {
            if (!result) return;
            const label = result.type === "image"
              ? `已复制图片${result.count > 1 ? `（含 ${result.count} 项链接）` : ""}`
              : `已复制${result.count > 1 ? ` ${result.count} 条` : ""}链接`;
            setNotice(label);
          })
          .catch((err) => {
            setError(`复制失败: ${err instanceof Error ? err.message : String(err)}`);
          });
        return;
      }

      // --- Space: detail open / media toggle ---
      if (e.key !== " ") return;
      if (isInput) return;
      const { detail: d, focusedId: fid, assets: list, view: v } = keyCtxRef.current;
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

  // If the open collection itself is deleted, it drops out of the reloaded list
  // (deleting a collection never cascades to its children — an orphaned child
  // re-parents to root instead, so viewing a child while its ancestor is deleted
  // never triggers this). Leave the now-dangling detail view for the last browse layout.
  useEffect(() => {
    if (
      view === "collection" &&
      activeCollectionId &&
      !collections.list.some((c) => c.id === activeCollectionId)
    ) {
      setView(prevBrowse.current);
    }
  }, [view, activeCollectionId, collections.list, setView]);

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
        activeFolder={activeQuery.folderId}
        activeTag={activeQuery.tagId}
        boardsActive={view === "boards" || view === "board"}
        onPickFolder={view === "board" ? bpq.pickFolder : lq.setFolder}
        onPickTag={view === "board" ? bpq.pickTag : lq.setTag}
        onViewBoards={() => changeView("boards")}
        onCreateFolder={(n) => void lib.createFolder(n)}
        onSetFolderColor={(id, color) => void lib.updateFolder(id, { color })}
        onDeleteFolder={(id) => void lib.deleteFolder(id)}
        onCreateTag={(n) => void lib.createTag(n)}
        onDeleteTag={(id) => void lib.deleteTag(id)}
        collections={collections.tree}
        activeCollectionId={view === "collection" ? activeCollectionId : null}
        onPickCollection={openCollection}
        onCreateCollection={(name, parentId) => void collections.createCollection(name, parentId)}
        onDeleteCollection={(id) => void collections.deleteCollection(id)}
        onDropAssetToCollection={(collectionId, assetId) =>
          void dropOnCollection(collectionId, assetId)
        }
        missingCount={lib.missingCount}
        onPickMissing={setMissing}
        activeMissing={view === "missing"}
        kindCounts={kindCounts}
        activeKinds={activeQuery.kind}
        minRating={activeQuery.minRating}
        favorite={activeQuery.favorite}
        onToggleKind={view === "board" ? bpq.toggleKind : lq.toggleKind}
        onSetRating={view === "board" ? bpq.setRating : lq.setRating}
        onSetFavorite={view === "board" ? bpq.setFavorite : lq.setFavorite}
        activeShapes={activeQuery.shapes}
        onToggleShape={view === "board" ? bpq.toggleShape : lq.toggleShape}
        minWidth={activeQuery.minWidth}
        maxWidth={activeQuery.maxWidth}
        minHeight={activeQuery.minHeight}
        maxHeight={activeQuery.maxHeight}
        onSetDimensions={view === "board" ? bpq.setDimensions : lq.setDimensions}
        minSize={activeQuery.minSize}
        maxSize={activeQuery.maxSize}
        onSetFileSize={view === "board" ? bpq.setFileSize : lq.setFileSize}
        timeDuration={{
          minDuration: activeQuery.minDuration,
          maxDuration: activeQuery.maxDuration,
          onSetDuration: view === "board" ? bpq.setDuration : lq.setDuration,
          createdAfter: activeQuery.createdAfter,
          createdBefore: activeQuery.createdBefore,
          onSetCreatedRange: view === "board" ? bpq.setCreatedRange : lq.setCreatedRange,
          indexedAfter: activeQuery.indexedAfter,
          indexedBefore: activeQuery.indexedBefore,
          onSetIndexedRange: view === "board" ? bpq.setIndexedRange : lq.setIndexedRange,
        }}
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
            boardQuery={bpq.boardQuery}
            setKeyword={bpq.setKeyword}
            onBack={() => changeView("boards")}
            onError={setError}
          />
        ) : view === "collection" && activeCollection ? (
          <CollectionView
            collection={activeCollection}
            refreshSignal={colRefresh}
            onBack={() => setView(prevBrowse.current)}
            onOpenDetail={setDetail}
            onSetCover={(assetId) =>
              void collections.updateCollection(activeCollection.id, { cover: assetId })
            }
            onMembersChanged={() => collections.reload()}
            onError={setError}
          />
        ) : (
          <>
            {view === "trash" && (
              <TrashView count={lib.trashCount} onEmpty={() => void run(() => api.purgeTrash(0))()} />
            )}
            <div className={styles.gridWrap} {...dropHandlers}>
              {dragging && (
                <div className={styles.dropHint} data-testid="drop-overlay">
                  松开以导入到当前库
                </div>
              )}
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
                  onFavorite={favorite}
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
                  onFavorite={favorite}
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
                  onFavorite={favorite}
                  onDetail={openDetail}
                  {...(activeQuery.folderId
                    ? {
                        onSetFolderCover: (id: string) =>
                          void lib.updateFolder(activeQuery.folderId as string, { cover: id }),
                      }
                    : {})}
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
          onFavorite={(fav) => void run((t) => api.favorite(t, fav), [inspectedAsset.id])()}
          onColorSearch={lq.setColor}
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
          onFavorite={(fav) => void run((t) => api.favorite(t, fav))()}
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
          onNotice={setNotice}
          onError={setError}
        />
      )}

      <AssetDetail
        asset={detail}
        toggleRef={videoToggleRef}
        onColorSearch={(hex) => {
          // Close the modal so the color-filtered grid behind it is visible.
          lq.setColor(hex);
          videoToggleRef.current = null;
          setDetail(null);
        }}
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
