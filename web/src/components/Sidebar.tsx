import { useState } from "react";
import type { AssetKind, AssetShape, Folder, KindCount, Tag } from "../types";
import type { CollectionNode } from "../hooks/useCollections";
import { FilterFacets } from "./FilterFacets";
import { SidebarCollections } from "./SidebarCollections";
import { AddForm } from "./SidebarAddForm";
import { IconAlert, IconBoard, IconFolder, IconGrid, IconPlus, IconTag, IconTrash } from "./icons";
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
  onDeleteFolder: (id: string) => void;
  onCreateTag: (name: string) => void;
  onDeleteTag: (id: string) => void;
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
  onToggleKind: (kind: AssetKind) => void;
  onSetRating: (rating: number) => void;
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
  onClearFilters: () => void;
}

export function Sidebar(p: Props) {
  const [addFolder, setAddFolder] = useState(false);
  const [addTag, setAddTag] = useState(false);
  const allActive =
    !p.activeFolder &&
    !p.activeTag &&
    !p.boardsActive &&
    !p.activeMissing &&
    p.activeKinds.length === 0 &&
    p.minRating === 0;

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
          <button
            key={f.id}
            className={`${styles.item} ${p.activeFolder === f.id ? styles.active : ""}`}
            title={f.path || f.name}
            onClick={() => p.onPickFolder(f.id)}
          >
            <IconFolder width={15} height={15} />
            <span className={styles.txt}>{f.name}</span>
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
          </button>
        ))}
      </div>

      <div className={styles.section}>
        <div className={styles.head}>
          <span>标签</span>
          <button className={styles.add} title="新建标签" onClick={() => setAddTag((x) => !x)}>
            <IconPlus width={13} height={13} />
          </button>
        </div>
        {addTag && (
          <AddForm
            placeholder="标签名称"
            onSubmit={(v) => {
              p.onCreateTag(v);
              setAddTag(false);
            }}
          />
        )}
        {p.tags.map((t) => (
          <button
            key={t.id}
            className={`${styles.item} ${p.activeTag === t.id ? styles.active : ""}`}
            onClick={() => p.onPickTag(t.id)}
          >
            {t.color ? (
              <i className={styles.swatch} style={{ background: t.color }} />
            ) : (
              <IconTag width={14} height={14} />
            )}
            <span className={styles.txt}>{t.name}</span>
            <span
              className={styles.del}
              title="删除"
              onClick={(e) => {
                e.stopPropagation();
                p.onDeleteTag(t.id);
              }}
            >
              <IconTrash width={13} height={13} />
            </span>
          </button>
        ))}
      </div>

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
        onToggleKind={p.onToggleKind}
        onSetRating={p.onSetRating}
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
      />
    </aside>
  );
}
