import { useCallback, useRef, useState } from "react";
import { type AssetKind, type AssetShape, EMPTY_QUERY, type Query } from "../types";

export interface LibraryQuery {
  query: Query;
  hasFilter: boolean;
  reset: () => void;
  setFolder: (folderId: string | null) => void;
  setTag: (tagId: string | null) => void;
  toggleKind: (kind: AssetKind) => void;
  setRating: (rating: number) => void;
  toggleShape: (shape: AssetShape) => void;
  setDimensions: (minWidth: number, maxWidth: number, minHeight: number, maxHeight: number) => void;
  setFileSize: (minSize: number, maxSize: number) => void;
  clearFilters: () => void;
  setKeyword: (keyword: string) => void;
  setColor: (hex: string | null) => void;
}

// Owns the composable library filter/search query (issue #75). folder/tag/kind/
// minRating merge and AND together server-side, so picking one facet never drops
// the others; keyword/color are the two mutually exclusive search modes. Facet
// changes run `onFilter` (the composer restores a browse layout); keyword/color
// edits preserve the current view, matching the topbar's in-place search.
//
// Every returned callback is referentially stable (useCallback). This is load-
// bearing: SearchBar's keyword debounce lists onKeyword in its effect deps, so an
// unstable setter re-ran that effect on every render and fired onKeyword("") in a
// ~300ms loop — which cleared a color picked from a palette swatch right after it
// applied ("回闪没", issue #88). onFilter is read through a ref so the setters stay
// stable even when the caller passes a fresh onFilter each render.
export function useLibraryQuery(onFilter: () => void): LibraryQuery {
  const [query, setQuery] = useState<Query>(EMPTY_QUERY);

  const onFilterRef = useRef(onFilter);
  onFilterRef.current = onFilter;

  const filter = useCallback((next: (q: Query) => Query) => {
    onFilterRef.current();
    setQuery(next);
  }, []);

  const reset = useCallback(() => setQuery(EMPTY_QUERY), []);
  const setFolder = useCallback(
    (folderId: string | null) => filter((q) => ({ ...q, folderId })),
    [filter],
  );
  const setTag = useCallback(
    (tagId: string | null) => filter((q) => ({ ...q, tagId })),
    [filter],
  );
  const toggleKind = useCallback(
    (kind: AssetKind) =>
      filter((q) => ({
        ...q,
        kind: q.kind.includes(kind) ? q.kind.filter((k) => k !== kind) : [...q.kind, kind],
      })),
    [filter],
  );
  const setRating = useCallback(
    (minRating: number) => filter((q) => ({ ...q, minRating })),
    [filter],
  );
  const toggleShape = useCallback(
    (shape: AssetShape) =>
      filter((q) => ({
        ...q,
        shapes: q.shapes.includes(shape)
          ? q.shapes.filter((s) => s !== shape)
          : [...q.shapes, shape],
      })),
    [filter],
  );
  const setDimensions = useCallback(
    (minWidth: number, maxWidth: number, minHeight: number, maxHeight: number) =>
      filter((q) => ({ ...q, minWidth, maxWidth, minHeight, maxHeight })),
    [filter],
  );
  const setFileSize = useCallback(
    (minSize: number, maxSize: number) => filter((q) => ({ ...q, minSize, maxSize })),
    [filter],
  );
  const clearFilters = useCallback(() => filter(() => EMPTY_QUERY), [filter]);
  // An empty keyword is not a search, so it must not clear an active color filter:
  // otherwise the keyword debounce firing onKeyword("") wipes a color picked from
  // a palette swatch (issue #88). A real keyword still switches off color search.
  const setKeyword = useCallback(
    (keyword: string) =>
      setQuery((q) => ({ ...q, keyword, colorHex: keyword.trim() ? null : q.colorHex })),
    [],
  );
  const setColor = useCallback(
    (colorHex: string | null) => setQuery((q) => ({ ...q, colorHex, keyword: "" })),
    [],
  );

  const hasFilter = Boolean(
    query.keyword ||
      query.colorHex ||
      query.folderId ||
      query.tagId ||
      query.kind.length > 0 ||
      query.minRating > 0 ||
      query.shapes.length > 0 ||
      query.minWidth > 0 ||
      query.maxWidth > 0 ||
      query.minHeight > 0 ||
      query.maxHeight > 0 ||
      query.minSize > 0 ||
      query.maxSize > 0,
  );

  return {
    query,
    hasFilter,
    reset,
    setFolder,
    setTag,
    toggleKind,
    setRating,
    toggleShape,
    setDimensions,
    setFileSize,
    clearFilters,
    setKeyword,
    setColor,
  };
}
