import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { KindCount, Query } from "../types";

// Fetches the format-facet counts for the current folder/tag/rating scope. The
// active kind selection and the keyword are intentionally excluded: the server
// ignores kind so every format stays selectable while multi-selecting, and the
// counts track the facet scope rather than the search text. A `version` bump
// refetches after mutations that change the library. Shared by the main sidebar
// and the board asset panel (issue #75).
export function useFacets(query: Query, version: number): KindCount[] {
  const [kinds, setKinds] = useState<KindCount[]>([]);

  useEffect(() => {
    let alive = true;
    api
      .facets({ folder: query.folderId, tag: query.tagId, rating: query.minRating })
      .then((f) => {
        if (alive) setKinds(f.kinds);
      })
      .catch(() => {
        if (alive) setKinds([]);
      });
    return () => {
      alive = false;
    };
  }, [query.folderId, query.tagId, query.minRating, version]);

  return kinds;
}
