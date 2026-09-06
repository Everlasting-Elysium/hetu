import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { Asset, Query, ViewMode } from "../types";

// Cross-references asset tags client-side: the API exposes tags per asset, so
// tag filtering resolves the tagged asset ids once, then intersects.
async function idsForTag(tagId: string, pool: Asset[]): Promise<Set<string>> {
  const results = await Promise.all(
    pool.map(async (a) => ((await api.assetTags(a.id)).some((t) => t.id === tagId) ? a.id : null)),
  );
  return new Set(results.filter((id): id is string => id !== null));
}

export interface AssetsState {
  assets: Asset[];
  loading: boolean;
  error: string | null;
}

// Resolves the active view + query into a concrete asset list. Keyword/color
// search short-circuit filters; otherwise library assets are filtered by
// folder and tag in memory. `version` bumps force a refetch after mutations.
export function useAssets(view: ViewMode, query: Query, version: number): AssetsState {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const reqId = useRef(0);

  useEffect(() => {
    const id = ++reqId.current;
    setLoading(true);
    setError(null);

    (async (): Promise<Asset[]> => {
      if (view === "boards" || view === "board") return [];
      if (view === "trash") return api.listTrash();
      if (view === "missing") return api.listMissing();
      if (query.colorHex) return api.searchColor(query.colorHex);
      if (query.keyword.trim()) return api.searchKeyword(query.keyword.trim());

      let list = await api.listAssets();
      if (query.folderId) list = list.filter((a) => a.folder_id === query.folderId);
      if (query.tagId) {
        const ids = await idsForTag(query.tagId, list);
        list = list.filter((a) => ids.has(a.id));
      }
      return list;
    })()
      .then((list) => {
        if (id === reqId.current) setAssets(list);
      })
      .catch((e: unknown) => {
        if (id === reqId.current) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (id === reqId.current) setLoading(false);
      });
  }, [view, query, version]);

  return { assets, loading, error };
}
