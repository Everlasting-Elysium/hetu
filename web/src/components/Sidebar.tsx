import { useState } from "react";
import type { AssetKind, AssetShape, Folder, KindCount, Tag } from "../types";
import type { CollectionNode } from "../hooks/useCollections";
import { thumbUrl } from "../api/client";
import { FilterFacets } from "./FilterFacets";
import { SidebarCollections } from "./SidebarCollections";
import { AddForm } from "./SidebarAddForm";
import { ColorPopover } from "./ColorPicker";
import type { TimeDurationFacetsProps } from "./TimeDurationFacets";
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

// tagDragMime isolates tag→tag merge drags from the asset→collection drags,
// which travel as "text/plain" (see SidebarCollections). Using a distinct key
// means a dragged tag can never be accidentally dropped onto a collection.
const tagDragMime = "application/x-hetu-tag";

export function Sidebar(p: Props) {
  const [addFolder, setAddFolder] = useState(false);
  const [addTag, setAddTag] = useState(false);
  const [colorFolder, setColorFolder] = useState<string | null>(null);
  const [dragTagId, setDragTagId] = useState<string | null>(null);
  const [dragOverTagId, setDragOverTagId] = useState<string | null>(null);
  // Pending merge awaiting confirmation: from tag folds INTO to tag (destructive).
  const [mergeConfirm, setMergeConfirm] = useState<{ from: Tag; to: Tag } | null>(null);
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
            className={`${styles.item} ${p.activeTag === t.id ? styles.active : ""} ${
              dragOverTagId === t.id ? styles.dragOver : ""
            }`}
            data-testid="tag-node"
            data-tag-id={t.id}
            draggable
            onClick={() => p.onPickTag(t.id)}
            onDragStart={(e) => {
              e.dataTransfer.setData(tagDragMime, t.id);
              e.dataTransfer.effectAllowed = "move";
              setDragTagId(t.id);
            }}
            onDragEnd={() => {
              setDragTagId(null);
              setDragOverTagId(null);
            }}
            onDragOver={(e) => {
              // Only accept another tag as a merge target, never itself.
              if (dragTagId && dragTagId !== t.id) {
                e.preventDefault();
                setDragOverTagId(t.id);
              }
            }}
            onDragLeave={() => setDragOverTagId((cur) => (cur === t.id ? null : cur))}
            onDrop={(e) => {
              e.preventDefault();
              setDragOverTagId(null);
              const fromId = e.dataTransfer.getData(tagDragMime);
              const from = p.tags.find((x) => x.id === fromId);
              if (from && fromId !== t.id) setMergeConfirm({ from, to: t });
            }}
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

      {mergeConfirm && (
        <div className={styles.confirmBackdrop} onMouseDown={() => setMergeConfirm(null)}>
          <div
            className={styles.confirmDialog}
            role="dialog"
            aria-label="合并标签"
            data-testid="tag-merge-confirm"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className={styles.confirmTitle}>合并标签</div>
            <p className={styles.confirmBody}>
              将标签「{mergeConfirm.from.name}」合并进「{mergeConfirm.to.name}」？
              <br />
              所有带「{mergeConfirm.from.name}」的素材都会改挂到「{mergeConfirm.to.name}」，
              该标签将被删除。此操作作用于全部素材且不可撤销。
            </p>
            <div className={styles.confirmActions}>
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => setMergeConfirm(null)}
              >
                取消
              </button>
              <button
                type="button"
                className="btn btn-danger"
                data-testid="tag-merge-confirm-ok"
                onClick={() => {
                  p.onMergeTags(mergeConfirm.from.id, mergeConfirm.to.id);
                  setMergeConfirm(null);
                }}
              >
                合并并删除
              </button>
            </div>
          </div>
        </div>
      )}

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
