import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { Folder, Tag } from "../types";

export interface Library {
  folders: Folder[];
  tags: Tag[];
  trashCount: number;
  refreshTrash: () => void;
  createFolder: (name: string) => Promise<void>;
  deleteFolder: (id: string) => Promise<void>;
  createTag: (name: string) => Promise<void>;
  deleteTag: (id: string) => Promise<void>;
}

// Owns sidebar data (folders + tags) and the trash badge count, plus their
// mutations. Kept separate from asset browsing so App stays a thin composer.
export function useLibrary(onError: (msg: string) => void): Library {
  const [folders, setFolders] = useState<Folder[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [trashCount, setTrashCount] = useState(0);

  const guard = useCallback(
    async (fn: () => Promise<void>) => {
      try {
        await fn();
      } catch (e) {
        onError(e instanceof Error ? e.message : String(e));
      }
    },
    [onError],
  );

  const loadFolders = useCallback(() => guard(async () => setFolders(await api.listFolders())), [guard]);
  const loadTags = useCallback(() => guard(async () => setTags(await api.listTags())), [guard]);
  const refreshTrash = useCallback(
    () => guard(async () => setTrashCount((await api.listTrash()).length)),
    [guard],
  );

  useEffect(() => {
    void loadFolders();
    void loadTags();
    void refreshTrash();
  }, [loadFolders, loadTags, refreshTrash]);

  return {
    folders,
    tags,
    trashCount,
    refreshTrash,
    createFolder: (name) => guard(async () => { await api.createFolder({ name, path: name }); await loadFolders(); }),
    deleteFolder: (id) => guard(async () => { await api.deleteFolder(id); await loadFolders(); }),
    createTag: (name) => guard(async () => { await api.createTag({ name }); await loadTags(); }),
    deleteTag: (id) => guard(async () => { await api.deleteTag(id); await loadTags(); }),
  };
}
