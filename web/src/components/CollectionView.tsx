import { useCallback, useEffect, useRef, useState } from "react";
import type { Asset, Collection, CollectionItem } from "../types";
import { api } from "../api/client";
import { IconArrowLeft } from "./icons";
import { CollectionMemberCard } from "./CollectionMemberCard";
import styles from "./CollectionView.module.css";

interface Props {
  collection: Collection;
  onBack: () => void;
  // Given a full Asset (resolved here via GET /assets/:id), opens the shared
  // AssetDetail modal owned by App.
  onOpenDetail: (asset: Asset) => void;
  onSetCover: (assetId: string) => void;
  // Fired after a member add/remove/reorder so App reloads the collection list —
  // the sidebar's resolved cover thumbnail can shift when membership/order change.
  onMembersChanged: () => void;
  onError: (msg: string) => void;
  // Bumped by App when an asset is dropped onto this (active) collection's sidebar
  // node, so the member grid refetches without a full remount.
  refreshSignal: number;
}

const errMsg = (e: unknown): string => (e instanceof Error ? e.message : String(e));

// Moves the member identified by srcId into targetId's slot, returning a new
// array (or the same reference when the move is a no-op). Dragging downward lands
// the source after the target, upward before it — the intuitive drop semantics.
function reorderMembers(
  items: CollectionItem[],
  srcId: string,
  targetId: string,
): CollectionItem[] {
  const from = items.findIndex((it) => it.asset_id === srcId);
  const to = items.findIndex((it) => it.asset_id === targetId);
  if (from < 0 || to < 0 || from === to) return items;
  const next = items.slice();
  const [moved] = next.splice(from, 1);
  if (!moved) return items;
  next.splice(to, 0, moved);
  return next;
}

// The collection detail view (ViewMode "collection"): a member thumbnail grid
// mirroring BoardCanvas's back-to-list chrome. Members come pre-enriched from
// GET /collections/:id/items (no per-asset request). Same-list drag reorders and
// persists the full new order; each member can be removed or set as the cover;
// clicking a member resolves its full Asset and opens the shared detail modal.
export function CollectionView({
  collection,
  onBack,
  onOpenDetail,
  onSetCover,
  onMembersChanged,
  onError,
  refreshSignal,
}: Props) {
  const [items, setItems] = useState<CollectionItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [dragOverId, setDragOverId] = useState<string | null>(null);
  // Fallback source id for browsers that withhold dataTransfer.getData outside
  // the drop handler; the drop payload (text/plain) remains the primary source.
  const dragId = useRef<string | null>(null);

  const collectionId = collection.id;

  useEffect(() => {
    let alive = true;
    setLoading(true);
    api
      .listCollectionItems(collectionId)
      .then((list) => {
        if (alive) setItems(list);
      })
      .catch((e: unknown) => {
        if (alive) onError(errMsg(e));
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [collectionId, refreshSignal, onError]);

  // On a failed persist (network error, or a 400 because another client changed
  // membership since this grid loaded — ErrCollectionItemsMismatch), the local
  // optimistic order must not be left standing: re-fetch the server's actual
  // order so the grid never diverges from what reorderCollectionItems rejected.
  const persistOrder = useCallback(
    async (next: CollectionItem[]) => {
      try {
        await api.reorderCollectionItems(collectionId, next.map((it) => it.asset_id));
        onMembersChanged();
      } catch (e) {
        onError(errMsg(e));
        try {
          setItems(await api.listCollectionItems(collectionId));
        } catch {
          // Resync itself failed (e.g. offline) — leave the optimistic order; the
          // effect's refreshSignal-driven refetch will reconcile on next change.
        }
      }
    },
    [collectionId, onMembersChanged, onError],
  );

  // itemsRef mirrors `items` so handleReorder reads the latest list without a
  // stale closure, while keeping the async persistOrder call outside the
  // setItems updater — an updater must stay a pure, synchronous reducer, and
  // React (StrictMode) may invoke it more than once per state change.
  const itemsRef = useRef(items);
  itemsRef.current = items;

  const handleReorder = useCallback(
    (srcId: string, targetId: string) => {
      const next = reorderMembers(itemsRef.current, srcId, targetId);
      if (next === itemsRef.current) return;
      setItems(next);
      void persistOrder(next);
    },
    [persistOrder],
  );

  const openDetail = useCallback(
    async (assetId: string) => {
      try {
        onOpenDetail(await api.getAsset(assetId));
      } catch (e) {
        onError(errMsg(e));
      }
    },
    [onOpenDetail, onError],
  );

  const remove = useCallback(
    async (assetId: string) => {
      try {
        await api.removeCollectionItem(collectionId, assetId);
        setItems((prev) => prev.filter((it) => it.asset_id !== assetId));
        onMembersChanged();
      } catch (e) {
        onError(errMsg(e));
      }
    },
    [collectionId, onMembersChanged, onError],
  );

  return (
    <div className={styles.view}>
      <div className={styles.toolbar}>
        <button className={styles.back} title="返回" onClick={onBack}>
          <IconArrowLeft width={16} height={16} /> 返回
        </button>
        <span className={styles.title}>{collection.name}</span>
        <span className={styles.count}>{items.length} 项</span>
      </div>

      <div className={styles.body}>
        {loading && items.length === 0 ? (
          <div className={styles.hint}>加载中…</div>
        ) : items.length === 0 ? (
          <div className={styles.hint}>合集为空。把素材拖到左侧此合集即可加入。</div>
        ) : (
          <div className={styles.grid} data-testid="collection-grid">
            {items.map((it) => (
              <CollectionMemberCard
                key={it.asset_id}
                item={it}
                isCover={collection.cover === it.asset_id}
                dragOver={dragOverId === it.asset_id}
                onOpen={() => void openDetail(it.asset_id)}
                onRemove={() => void remove(it.asset_id)}
                onSetCover={() => onSetCover(it.asset_id)}
                onDragStart={(e) => {
                  dragId.current = it.asset_id;
                  e.dataTransfer.setData("text/plain", it.asset_id);
                  e.dataTransfer.effectAllowed = "move";
                }}
                onDragOver={(e) => {
                  e.preventDefault();
                  setDragOverId(it.asset_id);
                }}
                onDragEnd={() => {
                  dragId.current = null;
                  setDragOverId(null);
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  const srcId = e.dataTransfer.getData("text/plain") || dragId.current || "";
                  setDragOverId(null);
                  if (srcId) handleReorder(srcId, it.asset_id);
                }}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
