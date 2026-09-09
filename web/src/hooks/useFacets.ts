import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { KindCount, Query } from "../types";

// Fetches the format-facet counts for the current folder/tag/rating/shape/
// dimension/size scope. The active kind selection and the keyword are
// intentionally excluded: the server ignores kind so every format stays
// selectable while multi-selecting, and the counts track the facet scope rather
// than the search text. Shape/dimensions/size DO narrow the scope like rating —
// they answer "how many of each format match my other filters", not "which
// format", so passing them keeps the counts honest (issue #101). A `version`
// bump refetches after mutations that change the library. Shared by the main
// sidebar and the board asset panel (issue #75).
export function useFacets(query: Query, version: number): KindCount[] {
  const [kinds, setKinds] = useState<KindCount[]>([]);

  useEffect(() => {
    let alive = true;
    api
      .facets({
        folder: query.folderId,
        tag: query.tagId,
        rating: query.minRating,
        shape: query.shapes,
        minWidth: query.minWidth,
        maxWidth: query.maxWidth,
        minHeight: query.minHeight,
        maxHeight: query.maxHeight,
        minSize: query.minSize,
        maxSize: query.maxSize,
      })
      .then((f) => {
        if (alive) setKinds(f.kinds);
      })
      .catch(() => {
        if (alive) setKinds([]);
      });
    return () => {
      alive = false;
    };
  }, [
    query.folderId,
    query.tagId,
    query.minRating,
    query.shapes,
    query.minWidth,
    query.maxWidth,
    query.minHeight,
    query.maxHeight,
    query.minSize,
    query.maxSize,
    version,
  ]);

  return kinds;
}
