import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type CollectionPatch } from "../api/client";
import type { Collection, NewCollection } from "../types";

const errMsg = (e: unknown): string => (e instanceof Error ? e.message : String(e));

// A collection plus its resolved children, for nested sidebar rendering.
export interface CollectionNode extends Collection {
  children: CollectionNode[];
}

// Pure: folds the flat parent_id-linked list into a forest. A collection is a
// root when its parent_id is "" or points at a parent absent from the list
// (defensive against an orphaned reference). Child order follows the input order
// (the API returns collections in a stable server-side order). Exported so it can
// be unit-tested and reused without instantiating the hook.
export function buildCollectionTree(list: Collection[]): CollectionNode[] {
  const nodes = new Map<string, CollectionNode>(
    list.map((c) => [c.id, { ...c, children: [] }]),
  );
  const roots: CollectionNode[] = [];
  for (const node of nodes.values()) {
    const parent = node.parent_id ? nodes.get(node.parent_id) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  return roots;
}

export interface Collections {
  list: Collection[];
  tree: CollectionNode[];
  reload: () => void;
  createCollection: (name: string, parentId: string) => Promise<Collection | null>;
  updateCollection: (id: string, patch: CollectionPatch) => Promise<void>;
  deleteCollection: (id: string) => Promise<void>;
  addItem: (collectionId: string, assetId: string) => Promise<boolean>;
}

// Owns the collection tree + its mutations, mirroring useLibrary/useBoards so App
// stays a thin composer. Single-collection member editing lives in CollectionView.
// Every mutation reloads the list so the sidebar's resolved cover thumbnails stay
// current (adding/removing/reordering members can shift the ord-smallest fallback).
export function useCollections(onError: (msg: string) => void): Collections {
  const [list, setList] = useState<Collection[]>([]);

  const guard = useCallback(
    async (fn: () => Promise<void>) => {
      try {
        await fn();
      } catch (e) {
        onError(errMsg(e));
      }
    },
    [onError],
  );

  const reload = useCallback(
    () => guard(async () => setList(await api.listCollections())),
    [guard],
  );

  useEffect(() => {
    void reload();
  }, [reload]);

  const createCollection = useCallback(
    async (name: string, parentId: string): Promise<Collection | null> => {
      try {
        // exactOptionalPropertyTypes: omit parent_id entirely for a root-level
        // collection rather than passing an explicit "" — the backend treats a
        // missing parent_id as root anyway (issue #55 contract).
        const payload: NewCollection = parentId ? { name, parent_id: parentId } : { name };
        const created = await api.createCollection(payload);
        await reload();
        return created;
      } catch (e) {
        onError(errMsg(e));
        return null;
      }
    },
    [reload, onError],
  );

  const addItem = useCallback(
    async (collectionId: string, assetId: string): Promise<boolean> => {
      try {
        await api.addCollectionItem(collectionId, assetId);
        await reload();
        return true;
      } catch (e) {
        onError(errMsg(e));
        return false;
      }
    },
    [reload, onError],
  );

  const tree = useMemo(() => buildCollectionTree(list), [list]);

  return {
    list,
    tree,
    reload,
    createCollection,
    addItem,
    updateCollection: (id, patch) =>
      guard(async () => {
        await api.updateCollection(id, patch);
        await reload();
      }),
    deleteCollection: (id) =>
      guard(async () => {
        await api.deleteCollection(id);
        await reload();
      }),
  };
}
