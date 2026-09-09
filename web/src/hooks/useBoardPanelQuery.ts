import { useCallback, useState } from "react";
import { type AssetKind, type AssetShape, EMPTY_QUERY, type Query } from "../types";

export interface BoardPanelQuery {
  boardQuery: Query;
  setKeyword: (keyword: string) => void;
  pickFolder: (folderId: string | null) => void;
  pickTag: (tagId: string | null) => void;
  toggleKind: (kind: AssetKind) => void;
  setRating: (rating: number) => void;
  toggleShape: (shape: AssetShape) => void;
  setDimensions: (minWidth: number, maxWidth: number, minHeight: number, maxHeight: number) => void;
  setFileSize: (minSize: number, maxSize: number) => void;
}

// Owns the board-local Query that scopes the board canvas's drag-source panel
// (BoardAssetPanel). This is App's board-context counterpart to
// useLibraryQuery: same Query shape, same facet mutators (folder / tag /
// format / star / shape / dimensions / size), but every setter here is a
// *plain* setState — none of them wrap the browse-restore side effect
// useLibraryQuery's setters carry (issue #75 wired that restore for the main
// library only). That distinction is load-bearing for issue #108: App routes
// the sidebar's facet controls to these setters while `view === "board"`, and
// picking a facet must narrow the panel in place, never navigate away from the
// canvas. Facet toggles mirror the main library: re-picking a tag clears it and
// toggling a kind/shape adds/removes it.
export function useBoardPanelQuery(): BoardPanelQuery {
  const [boardQuery, setBoardQuery] = useState<Query>(EMPTY_QUERY);
  const setKeyword = useCallback(
    (keyword: string) => setBoardQuery((q) => ({ ...q, keyword })),
    [],
  );
  const pickFolder = useCallback(
    (folderId: string | null) => setBoardQuery((q) => ({ ...q, folderId })),
    [],
  );
  const pickTag = useCallback(
    (tagId: string | null) =>
      setBoardQuery((q) => ({ ...q, tagId: tagId && q.tagId === tagId ? null : tagId })),
    [],
  );
  const toggleKind = useCallback(
    (kind: AssetKind) =>
      setBoardQuery((q) => ({
        ...q,
        kind: q.kind.includes(kind) ? q.kind.filter((k) => k !== kind) : [...q.kind, kind],
      })),
    [],
  );
  const setRating = useCallback(
    (rating: number) => setBoardQuery((q) => ({ ...q, minRating: rating })),
    [],
  );
  const toggleShape = useCallback(
    (shape: AssetShape) =>
      setBoardQuery((q) => ({
        ...q,
        shapes: q.shapes.includes(shape)
          ? q.shapes.filter((s) => s !== shape)
          : [...q.shapes, shape],
      })),
    [],
  );
  const setDimensions = useCallback(
    (minWidth: number, maxWidth: number, minHeight: number, maxHeight: number) =>
      setBoardQuery((q) => ({ ...q, minWidth, maxWidth, minHeight, maxHeight })),
    [],
  );
  const setFileSize = useCallback(
    (minSize: number, maxSize: number) => setBoardQuery((q) => ({ ...q, minSize, maxSize })),
    [],
  );
  return {
    boardQuery,
    setKeyword,
    pickFolder,
    pickTag,
    toggleKind,
    setRating,
    toggleShape,
    setDimensions,
    setFileSize,
  };
}
