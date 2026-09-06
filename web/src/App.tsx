import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api/client";
import {
  type Asset,
  type BrowseLayout,
  EMPTY_QUERY,
  isBrowseLayout,
  isLibraryView,
  type Query,
  type Tag,
  type ViewMode,
} from "./types";
import { useAssets } from "./hooks/useAssets";
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
  const [query, setQuery] = useState<Query>(EMPTY_QUERY);
  const [version, setVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [inspectorTags, setInspectorTags] = useState<Tag[]>([]);
  const [activeBoardId, setActiveBoardId] = useState<string | null>(null);
  const [detail, setDetail] = useState<Asset | null>(null);
  const [immersiveIndex, setImmersiveIndex] = useState(0);

  const bump = useCallback(() => setVersion((v) => v + 1), []);
  const lib = useLibrary(setError);
  const boards = useBoards(setError);
  const { assets, loading, error: loadErr } = useAssets(view, query, version);
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

  useEffect(() => {
    if (error) {
      const t = setTimeout(() => setError(null), 4000);
      return () => clearTimeout(t);
    }
  }, [error]);

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
        if (!override) sel.clear();
        bump();
        lib.refreshTrash();
        lib.refreshMissing();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [sel, bump, lib],
  );

  // Filtering keeps the current browse layout (or restores it from a special view).
  const applyFilter = (patch: Partial<Query>) => {
    setView(isBrowseLayout(view) ? view : prevBrowse.current);
    sel.clear();
    setQuery({ ...EMPTY_QUERY, ...patch });
  };
  const setFolder = (folderId: string | null) => applyFilter({ folderId });
  const setTag = (tagId: string | null) => applyFilter({ tagId });

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
      setQuery(EMPTY_QUERY);
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

  const emptyHint =
    view === "trash"
      ? "回收站是空的。"
      : view === "missing"
        ? "没有丢失文件，所有索引文件均可访问。"
        : query.keyword || query.colorHex
          ? "没有匹配的素材，换个条件试试。"
          : "运行 `bin/hetu scan` 索引素材目录后即可浏览。";

  const rate = (id: string, rating: number) => void run((t) => api.rate(t, rating), [id])();
  const color = (id: string, hex: string) => void run((t) => api.colorLabel(t, hex), [id])();
  const openDetail = (id: string) => setDetail(assets.find((a) => a.id === id) ?? null);

  return (
    <div className={`app ${isAssetView && inspectedAsset ? "inspect" : ""}`} onClick={() => sel.clear()}>
      <div className={brand.brand}>
        <span className={brand.logo}>河</span>
        <span className={brand.brandName}>
          hetu<small>DAM</small>
        </span>
      </div>

      <Sidebar
        folders={lib.folders}
        tags={lib.tags}
        activeFolder={query.folderId}
        activeTag={query.tagId}
        boardsActive={view === "boards" || view === "board"}
        onPickFolder={setFolder}
        onPickTag={setTag}
        onViewBoards={() => changeView("boards")}
        onCreateFolder={(n) => void lib.createFolder(n)}
        onDeleteFolder={(id) => void lib.deleteFolder(id)}
        onCreateTag={(n) => void lib.createTag(n)}
        onDeleteTag={(id) => void lib.deleteTag(id)}
        missingCount={lib.missingCount}
        onPickMissing={setMissing}
        activeMissing={view === "missing"}
      />

      {isAssetView && (
        <SearchBar
          view={view}
          trashCount={lib.trashCount}
          missingCount={lib.missingCount}
          onKeyword={(q) => setQuery((p) => ({ ...EMPTY_QUERY, folderId: p.folderId, tagId: p.tagId, keyword: q }))}
          onColor={(hex) => setQuery((p) => ({ ...EMPTY_QUERY, folderId: p.folderId, tagId: p.tagId, colorHex: hex }))}
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
          <BoardCanvas boardId={activeBoardId} onBack={() => changeView("boards")} onError={setError} />
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
                  onRate={rate}
                  onColor={color}
                />
              ) : view === "waterfall" ? (
                <WaterfallGrid
                  assets={assets}
                  loading={loading}
                  error={loadErr}
                  selection={sel}
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

      {isAssetView && inspectedAsset && (
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
          onClear={sel.clear}
          onTag={(tagId) => void run((t) => api.tag(t, [tagId]))()}
          onRate={(rating) => void run((t) => api.rate(t, rating))()}
          onColor={(hex) => void run((t) => api.colorLabel(t, hex))()}
          onMove={(folderId) => void run((t) => api.move(t, folderId))()}
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

      <AssetDetail asset={detail} onClose={() => setDetail(null)} />

      {error && <div className={styles.toast}>{error}</div>}
    </div>
  );
}
