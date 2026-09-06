import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api/client";
import { type Asset, EMPTY_QUERY, type Query, type Tag, type ViewMode } from "./types";
import { useAssets } from "./hooks/useAssets";
import { useLibrary } from "./hooks/useLibrary";
import { useSelection } from "./hooks/useSelection";
import { Sidebar } from "./components/Sidebar";
import { SearchBar } from "./components/SearchBar";
import { AssetGrid } from "./components/AssetGrid";
import { AssetDetail } from "./components/AssetDetail";
import { BatchBar } from "./components/BatchBar";
import { TrashView } from "./components/TrashView";
import { InspectorPanel } from "./components/InspectorPanel";
import brand from "./components/Sidebar.module.css";
import styles from "./App.module.css";

export default function App() {
  const [view, setView] = useState<ViewMode>("library");
  const [query, setQuery] = useState<Query>(EMPTY_QUERY);
  const [version, setVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [inspectorTags, setInspectorTags] = useState<Tag[]>([]);
  const [detail, setDetail] = useState<Asset | null>(null);

  const bump = useCallback(() => setVersion((v) => v + 1), []);
  const lib = useLibrary(setError);
  const { assets, loading, error: loadErr } = useAssets(view, query, version);
  const ids = useMemo(() => assets.map((a) => a.id), [assets]);
  const sel = useSelection(ids);

  // The inspector shows for a single selection; resolve it from the loaded list.
  const inspectedId = sel.count === 1 ? ([...sel.selected][0] ?? null) : null;
  const inspectedAsset = inspectedId
    ? assets.find((a) => a.id === inspectedId)
    : undefined;

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

  const setFolder = (folderId: string | null) => {
    setView("library");
    sel.clear();
    setQuery({ ...EMPTY_QUERY, folderId });
  };
  const setTag = (tagId: string | null) => {
    setView("library");
    sel.clear();
    setQuery({ ...EMPTY_QUERY, tagId });
  };
  const changeView = (v: ViewMode) => {
    setView(v);
    sel.clear();
    setQuery(EMPTY_QUERY);
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

  return (
    <div className={`app ${inspectedAsset ? "inspect" : ""}`} onClick={() => sel.clear()}>
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
        onPickFolder={setFolder}
        onPickTag={setTag}
        onCreateFolder={(n) => void lib.createFolder(n)}
        onDeleteFolder={(id) => void lib.deleteFolder(id)}
        onCreateTag={(n) => void lib.createTag(n)}
        onDeleteTag={(id) => void lib.deleteTag(id)}
        missingCount={lib.missingCount}
        onPickMissing={setMissing}
        activeMissing={view === "missing"}
      />

      <SearchBar
        view={view}
        trashCount={lib.trashCount}
        missingCount={lib.missingCount}
        onKeyword={(q) => setQuery((p) => ({ ...EMPTY_QUERY, folderId: p.folderId, tagId: p.tagId, keyword: q }))}
        onColor={(hex) => setQuery((p) => ({ ...EMPTY_QUERY, folderId: p.folderId, tagId: p.tagId, colorHex: hex }))}
        onViewChange={changeView}
      />

      <div className={styles.main} onClick={(e) => e.stopPropagation()}>
        {view === "trash" && (
          <TrashView
            count={lib.trashCount}
            onEmpty={() => void run(() => api.purgeTrash(0))()}
          />
        )}
        <div className={styles.gridWrap}>
          <AssetGrid
            assets={assets}
            loading={loading}
            error={loadErr}
            selection={sel}
            emptyHint={emptyHint}
            onRate={(id, rating) => void run((t) => api.rate(t, rating), [id])()}
            onColor={(id, hex) => void run((t) => api.colorLabel(t, hex), [id])()}
            onDetail={(id) => setDetail(assets.find((a) => a.id === id) ?? null)}
          />
        </div>
      </div>

      {inspectedAsset && (
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

      <AssetDetail asset={detail} onClose={() => setDetail(null)} />

      {error && <div className={styles.toast}>{error}</div>}
    </div>
  );
}
