import { useState } from "react";
import type { AssetKind, AssetShape, Folder, KindCount, Tag } from "../types";
import type { CollectionNode } from "../hooks/useCollections";
import { thumbUrl } from "../api/client";
import { FilterFacets } from "./FilterFacets";
import { SidebarCollections } from "./SidebarCollections";
import { SidebarTags } from "./SidebarTags";
import { AddForm } from "./SidebarAddForm";
import { ColorPopover } from "./ColorPicker";
import type { TimeDurationFacetsProps } from "./TimeDurationFacets";
import { IconAlert, IconBoard, IconFolder, IconGrid, IconPlus, IconTrash } from "./icons";
import styles from "./Sidebar.module.css";

interface Props {
  folders: Folder[];
  tags: Tag[];
  activeFolder: string | null;
  activeTag: string | null;
  boardsActive: boolean;
  onPickFolder: (id: string | null) => void;
  onPickTag: (id: string | null) => void;
  onViewBoards: () => void;
  onCreateFolder: (name: string) => void;
  onSetFolderColor: (id: string, color: string) => void;
  onDeleteFolder: (id: string) => void;
  onCreateTag: (name: string) => void;
  onDeleteTag: (id: string) => void;
  // Global tag merge (issue #62): fold fromId into toId across all assets and
  // delete fromId. Triggered by dragging one tag onto another; the Sidebar owns
  // the confirmation gate before this fires (destructive, irreversible).
  onMergeTags: (fromId: string, toId: string) => void;
  collections: CollectionNode[];
  activeCollectionId: string | null;
  onPickCollection: (id: string) => void;
  onCreateCollection: (name: string, parentId: string) => void;
  onDeleteCollection: (id: string) => void;
  onDropAssetToCollection: (collectionId: string, assetId: string) => void;
  missingCount: number;
  onPickMissing: () => void;
  activeMissing: boolean;
  kindCounts: KindCount[];
  activeKinds: AssetKind[];
  minRating: number;
  favorite: boolean;
  onToggleKind: (kind: AssetKind) => void;
  onSetRating: (rating: number) => void;
  onSetFavorite: (favorite: boolean) => void;
  activeShapes: AssetShape[];
  onToggleShape: (shape: AssetShape) => void;
  minWidth: number;
  maxWidth: number;
  minHeight: number;
  maxHeight: number;
  onSetDimensions: (minWidth: number, maxWidth: number, minHeight: number, maxHeight: number) => void;
  minSize: number;
  maxSize: number;
  onSetFileSize: (minSize: number, maxSize: number) => void;
  timeDuration: TimeDurationFacetsProps;
  onClearFilters: () => void;
}

export function Sidebar(p: Props) {
  const [addFolder, setAddFolder] = useState(false);
  const [colorFolder, setColorFolder] = useState<string | null>(null);
  const allActive =
    !p.activeFolder &&
    !p.activeTag &&
    !p.boardsActive &&
    !p.activeMissing &&
    p.activeKinds.length === 0 &&
    p.minRating === 0 &&
    !p.favorite;

  return (
    <aside className={styles.side}>
      <div className={styles.section}>
        <button
          className={`${styles.item} ${allActive ? styles.active : ""}`}
          onClick={() => p.onClearFilters()}
        >
          <IconGrid width={15} height={15} />
          <span className={styles.txt}>全部素材</span>
        </button>
        <button
          className={`${styles.item} ${p.boardsActive ? styles.active : ""}`}
          onClick={() => p.onViewBoards()}
        >
          <IconBoard width={15} height={15} />
          <span className={styles.txt}>图板</span>
        </button>
        {p.missingCount > 0 && (
          <button
            className={`${styles.item} ${p.activeMissing ? styles.active : ""}`}
            onClick={p.onPickMissing}
          >
            <IconAlert width={15} height={15} />
            <span className={styles.txt}>丢失文件</span>
            <span className={styles.badge}>{p.missingCount}</span>
          </button>
        )}
      </div>

      <div className={styles.section}>
        <div className={styles.head}>
          <span>文件夹</span>
          <button className={styles.add} title="新建文件夹" onClick={() => setAddFolder((x) => !x)}>
            <IconPlus width={13} height={13} />
          </button>
        </div>
        {addFolder && (
          <AddForm
            placeholder="文件夹名称"
            onSubmit={(v) => {
              p.onCreateFolder(v);
              setAddFolder(false);
            }}
          />
        )}
        {p.folders.map((f) => (
          <div
            key={f.id}
            className={`${styles.item} ${styles.node} ${p.activeFolder === f.id ? styles.active : ""}`}
            data-testid="folder-node"
            data-folder-id={f.id}
            title={f.path || f.name}
            onClick={() => p.onPickFolder(f.id)}
          >
            {f.cover_url ? (
              <img className={styles.cover} src={thumbUrl(f.cover)} alt="" draggable={false} />
            ) : (
              <IconFolder width={15} height={15} />
            )}
            <span className={styles.txt}>{f.name}</span>
            {f.color && (
              <i
                className={styles.swatch}
                data-testid="folder-color"
                style={{ background: f.color }}
              />
            )}
            <ColorPopover
              open={colorFolder === f.id}
              value={f.color}
              onPick={(hex) => {
                p.onSetFolderColor(f.id, hex);
                setColorFolder(null);
              }}
            >
              <span
                className={styles.act}
                title="文件夹颜色"
                data-testid="folder-color-btn"
                onClick={(e) => {
                  e.stopPropagation();
                  setColorFolder((cur) => (cur === f.id ? null : f.id));
                }}
              >
                <span className={styles.dot} style={f.color ? { background: f.color } : undefined} />
              </span>
            </ColorPopover>
            <span
              className={styles.del}
              title="删除"
              onClick={(e) => {
                e.stopPropagation();
                p.onDeleteFolder(f.id);
              }}
            >
              <IconTrash width={13} height={13} />
            </span>
          </div>
        ))}
      </div>

      <SidebarTags
        tags={p.tags}
        activeTag={p.activeTag}
        onPickTag={p.onPickTag}
        onCreateTag={p.onCreateTag}
        onDeleteTag={p.onDeleteTag}
        onMergeTags={p.onMergeTags}
      />

      <SidebarCollections
        nodes={p.collections}
        activeId={p.activeCollectionId}
        onPick={p.onPickCollection}
        onCreate={p.onCreateCollection}
        onDelete={p.onDeleteCollection}
        onDropAsset={p.onDropAssetToCollection}
      />

      <FilterFacets
        counts={p.kindCounts}
        activeKinds={p.activeKinds}
        minRating={p.minRating}
        favorite={p.favorite}
        onToggleKind={p.onToggleKind}
        onSetRating={p.onSetRating}
        onSetFavorite={p.onSetFavorite}
        activeShapes={p.activeShapes}
        onToggleShape={p.onToggleShape}
        minWidth={p.minWidth}
        maxWidth={p.maxWidth}
        minHeight={p.minHeight}
        maxHeight={p.maxHeight}
        onSetDimensions={p.onSetDimensions}
        minSize={p.minSize}
        maxSize={p.maxSize}
        onSetFileSize={p.onSetFileSize}
        timeDuration={p.timeDuration}
      />
    </aside>
  );
}
