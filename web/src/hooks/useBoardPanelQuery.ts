import { useCallback, useState } from "react";
import { type AssetKind, EMPTY_QUERY, type Query } from "../types";

export interface BoardPanelQuery {
  boardQuery: Query;
  setKeyword: (keyword: string) => void;
  pickTag: (tagId: string) => void;
  toggleKind: (kind: AssetKind) => void;
  setRating: (rating: number) => void;
}

// The drag-source panel reuses the main library's query model; this owns that
// board-local Query and its facet mutators (search / tag / format / star) so
// BoardCanvas stays a thin composer. Facet toggles mirror the main library:
// re-picking a tag clears it and toggling a kind adds/removes it (issue #75).
export function useBoardPanelQuery(): BoardPanelQuery {
  const [boardQuery, setBoardQuery] = useState<Query>(EMPTY_QUERY);
  const setKeyword = useCallback(
    (keyword: string) => setBoardQuery((q) => ({ ...q, keyword })),
    [],
  );
  const pickTag = useCallback(
    (tagId: string) => setBoardQuery((q) => ({ ...q, tagId: q.tagId === tagId ? null : tagId })),
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
  return { boardQuery, setKeyword, pickTag, toggleKind, setRating };
}
