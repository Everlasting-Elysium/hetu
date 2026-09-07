import { useState } from "react";
import { type AssetKind, EMPTY_QUERY, type Query } from "../types";

export interface LibraryQuery {
  query: Query;
  hasFilter: boolean;
  reset: () => void;
  setFolder: (folderId: string | null) => void;
  setTag: (tagId: string | null) => void;
  toggleKind: (kind: AssetKind) => void;
  setRating: (rating: number) => void;
  clearFilters: () => void;
  setKeyword: (keyword: string) => void;
  setColor: (hex: string | null) => void;
}

// Owns the composable library filter/search query (issue #75). folder/tag/kind/
// minRating merge and AND together server-side, so picking one facet never drops
// the others; keyword/color are the two mutually exclusive search modes. Facet
// changes run `onFilter` (the composer restores a browse layout); keyword/color
// edits preserve the current view, matching the topbar's in-place search.
export function useLibraryQuery(onFilter: () => void): LibraryQuery {
  const [query, setQuery] = useState<Query>(EMPTY_QUERY);

  const filter = (next: (q: Query) => Query) => {
    onFilter();
    setQuery(next);
  };

  return {
    query,
    hasFilter: Boolean(
      query.keyword ||
        query.colorHex ||
        query.folderId ||
        query.tagId ||
        query.kind.length > 0 ||
        query.minRating > 0,
    ),
    reset: () => setQuery(EMPTY_QUERY),
    setFolder: (folderId) => filter((q) => ({ ...q, folderId })),
    setTag: (tagId) => filter((q) => ({ ...q, tagId })),
    toggleKind: (kind) =>
      filter((q) => ({
        ...q,
        kind: q.kind.includes(kind) ? q.kind.filter((k) => k !== kind) : [...q.kind, kind],
      })),
    setRating: (minRating) => filter((q) => ({ ...q, minRating })),
    clearFilters: () => filter(() => EMPTY_QUERY),
    setKeyword: (keyword) => setQuery((q) => ({ ...q, keyword, colorHex: null })),
    setColor: (colorHex) => setQuery((q) => ({ ...q, colorHex, keyword: "" })),
  };
}
