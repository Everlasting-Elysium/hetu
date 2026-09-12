import { useState } from "react";
import type { Tag } from "../types";
import { AddForm } from "./SidebarAddForm";
import { IconPlus, IconTag, IconTrash } from "./icons";
import styles from "./Sidebar.module.css";

interface Props {
  tags: Tag[];
  activeTag: string | null;
  onPickTag: (id: string | null) => void;
  onCreateTag: (name: string) => void;
  onDeleteTag: (id: string) => void;
  // Global tag merge (issue #62): fold fromId into toId across all assets and
  // delete fromId. Triggered by dragging one tag onto another; this component
  // owns the confirmation gate before it fires (destructive, irreversible).
  onMergeTags: (fromId: string, toId: string) => void;
}

// tagDragMime isolates tag→tag merge drags from the asset→collection drags,
// which travel as "text/plain" (see SidebarCollections). Using a distinct key
// means a dragged tag can never be accidentally dropped onto a collection.
const tagDragMime = "application/x-hetu-tag";

// The sidebar "标签" section: a flat, create/delete tag list where dragging one
// tag onto another folds it in (a confirmed, global, destructive merge).
// Extracted from Sidebar (mirroring SidebarCollections) to keep each file under
// the 250-LOC ceiling; owns its own add/drag/confirm state.
export function SidebarTags({ tags, activeTag, onPickTag, onCreateTag, onDeleteTag, onMergeTags }: Props) {
  const [addTag, setAddTag] = useState(false);
  const [dragTagId, setDragTagId] = useState<string | null>(null);
  const [dragOverTagId, setDragOverTagId] = useState<string | null>(null);
  // Pending merge awaiting confirmation: from tag folds INTO to tag (destructive).
  const [mergeConfirm, setMergeConfirm] = useState<{ from: Tag; to: Tag } | null>(null);

  return (
    <>
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
              onCreateTag(v);
              setAddTag(false);
            }}
          />
        )}
        {tags.map((t) => (
          <button
            key={t.id}
            className={`${styles.item} ${activeTag === t.id ? styles.active : ""} ${
              dragOverTagId === t.id ? styles.dragOver : ""
            }`}
            data-testid="tag-node"
            data-tag-id={t.id}
            draggable
            onClick={() => onPickTag(t.id)}
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
              const from = tags.find((x) => x.id === fromId);
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
                onDeleteTag(t.id);
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
              <button type="button" className="btn btn-ghost" onClick={() => setMergeConfirm(null)}>
                取消
              </button>
              <button
                type="button"
                className="btn btn-danger"
                data-testid="tag-merge-confirm-ok"
                onClick={() => {
                  onMergeTags(mergeConfirm.from.id, mergeConfirm.to.id);
                  setMergeConfirm(null);
                }}
              >
                合并并删除
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
