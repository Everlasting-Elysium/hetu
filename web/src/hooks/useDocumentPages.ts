import { useEffect, useState } from "react";
import type { DocumentPage } from "../types";
import { api } from "../api/client";

export interface DocumentPagesState {
  pages: DocumentPage[];
  loading: boolean;
  error: string | null;
}

// Fetches the per-page thumbnail index for an asset (GET /assets/{id}/pages,
// issue #48). A stale guard drops the result of a superseded id — the same
// pattern as the palette/tags effects in App/InspectorPanel — so a fast switch
// between assets never renders the wrong document's pages. An empty or
// single-page list is not an error: the caller falls back to the single-image
// preview, so a network failure is the only `error` a consumer surfaces.
export function useDocumentPages(id: string): DocumentPagesState {
  const [pages, setPages] = useState<DocumentPage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let stale = false;
    setLoading(true);
    setError(null);
    api
      .listAssetPages(id)
      .then((p) => {
        if (stale) return;
        setPages(p);
        setLoading(false);
      })
      .catch((e: unknown) => {
        if (stale) return;
        setError(e instanceof Error ? e.message : String(e));
        setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [id]);

  return { pages, loading, error };
}
