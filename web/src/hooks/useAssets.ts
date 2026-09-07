import { useEffect, useRef, useState } from "react";
import { api, queryFilter } from "../api/client";
import type { Asset, Query, ViewMode } from "../types";

export interface AssetsState {
  assets: Asset[];
  loading: boolean;
  error: string | null;
}

// Resolves the active view + query into a concrete asset list. Color and keyword
// search short-circuit; both keyword search and plain listing push the
// folder/tag/kind/rating facets to the server, so no asset list is filtered in
// memory (issue #75 — this replaces the per-asset assetTags() N+1). A `version`
// bump forces a refetch after mutations.
export function useAssets(view: ViewMode, query: Query, version: number): AssetsState {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const reqId = useRef(0);

  // Collapse the browse layouts (grid/waterfall/gallery/immersive) to one dataset
  // key so switching layout does not refetch — only a real dataset change does.
  const dataset = view === "trash" ? "trash" : view === "missing" ? "missing" : "library";

  useEffect(() => {
    const id = ++reqId.current;
    setLoading(true);
    setError(null);

    (async (): Promise<Asset[]> => {
      if (view === "boards" || view === "board") return [];
      if (dataset === "trash") return api.listTrash();
      if (dataset === "missing") return api.listMissing();
      if (query.colorHex) return api.searchColor(query.colorHex);
      const filter = queryFilter(query);
      if (query.keyword.trim()) return api.searchKeyword(query.keyword.trim(), filter);
      return api.listAssets(filter);
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
  }, [dataset, query, version]);

  return { assets, loading, error };
}
