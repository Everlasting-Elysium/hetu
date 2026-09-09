import { type ReactElement, useState } from "react";
import type { CollectionNode } from "../hooks/useCollections";
import { thumbUrl } from "../api/client";
import { AddForm } from "./SidebarAddForm";
import { IconChevronRight, IconCollection, IconPlus, IconTrash } from "./icons";
import styles from "./Sidebar.module.css";

interface Props {
  nodes: CollectionNode[];
  activeId: string | null;
  onPick: (id: string) => void;
  onCreate: (name: string, parentId: string) => void;
  onDelete: (id: string) => void;
  onDropAsset: (collectionId: string, assetId: string) => void;
}

// Depth indent step (px) so nested collections read as a tree. Nodes are expanded
// by default (collections are few); collapse state is local and unpersisted.
const INDENT = 14;

// The sidebar "合集" section: a recursive nested tree with expand/collapse, a
// root + per-node create entry, delete, and per-node drop targets that accept an
// asset dragged from any grid/card (payload = text/plain asset id). Rendering is a
// closure over the local collapse/drag/add state, so a single component owns the
// whole forest without prop-drilling those transient bits down each level.
export function SidebarCollections({ nodes, activeId, onPick, onCreate, onDelete, onDropAsset }: Props) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [dragOverId, setDragOverId] = useState<string | null>(null);
  const [addChildOf, setAddChildOf] = useState<string | null>(null);
  const [addRoot, setAddRoot] = useState(false);

  const toggle = (id: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const openChild = (id: string) => {
    setAddChildOf(id);
    // Reveal children so the new sub-collection is visible once created.
    setCollapsed((prev) => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  };

  const renderNode = (node: CollectionNode, depth: number): ReactElement => {
    const hasChildren = node.children.length > 0;
    const open = !collapsed.has(node.id);
    return (
      <div key={node.id}>
        <div
          className={`${styles.item} ${styles.node} ${activeId === node.id ? styles.active : ""} ${
            dragOverId === node.id ? styles.dragOver : ""
          }`}
          data-testid="collection-node"
          data-collection-id={node.id}
          style={{ paddingLeft: `calc(var(--sp-3) + ${depth * INDENT}px)` }}
          onClick={() => onPick(node.id)}
          onDragOver={(e) => {
            e.preventDefault();
            setDragOverId(node.id);
          }}
          onDragLeave={() => setDragOverId((cur) => (cur === node.id ? null : cur))}
          onDrop={(e) => {
            e.preventDefault();
            setDragOverId(null);
            const assetId = e.dataTransfer.getData("text/plain");
            if (assetId) onDropAsset(node.id, assetId);
          }}
        >
          <button
            type="button"
            className={styles.twist}
            style={{ visibility: hasChildren ? "visible" : "hidden" }}
            title={open ? "折叠" : "展开"}
            onClick={(e) => {
              e.stopPropagation();
              toggle(node.id);
            }}
          >
            <IconChevronRight
              width={12}
              height={12}
              style={{ transform: open ? "rotate(90deg)" : "none" }}
            />
          </button>
          {node.cover ? (
            <img className={styles.cover} src={thumbUrl(node.cover)} alt="" draggable={false} />
          ) : (
            <IconCollection width={15} height={15} />
          )}
          <span className={styles.txt}>{node.name}</span>
          <span
            className={styles.act}
            title="新建子合集"
            onClick={(e) => {
              e.stopPropagation();
              openChild(node.id);
            }}
          >
            <IconPlus width={12} height={12} />
          </span>
          <span
            className={styles.del}
            title="删除"
            onClick={(e) => {
              e.stopPropagation();
              onDelete(node.id);
            }}
          >
            <IconTrash width={13} height={13} />
          </span>
        </div>
        {addChildOf === node.id && (
          <div style={{ paddingLeft: `${(depth + 1) * INDENT}px` }}>
            <AddForm
              placeholder="子合集名称"
              onSubmit={(v) => {
                onCreate(v, node.id);
                setAddChildOf(null);
              }}
            />
          </div>
        )}
        {open && hasChildren && node.children.map((c) => renderNode(c, depth + 1))}
      </div>
    );
  };

  return (
    <div className={styles.section}>
      <div className={styles.head}>
        <span>合集</span>
        <button className={styles.add} title="新建合集" onClick={() => setAddRoot((x) => !x)}>
          <IconPlus width={13} height={13} />
        </button>
      </div>
      {addRoot && (
        <AddForm
          placeholder="合集名称"
          onSubmit={(v) => {
            onCreate(v, "");
            setAddRoot(false);
          }}
        />
      )}
      {nodes.map((n) => renderNode(n, 0))}
    </div>
  );
}
