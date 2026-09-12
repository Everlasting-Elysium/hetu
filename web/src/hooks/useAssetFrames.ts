import { useEffect, useState } from "react";
import type { SequenceFrame } from "../types";
import { api } from "../api/client";

export interface AssetFramesState {
  frames: SequenceFrame[];
  loading: boolean;
  error: string | null;
}

// Fetches the frame index for an asset (GET /assets/{id}/frames, issue #62). A
// stale guard drops the result of a superseded id — the same pattern as
// useDocumentPages — so a fast switch between assets never renders the wrong
// sequence. An empty or single-frame list is not an error: the caller falls back
// to the ordinary single-image preview, so a network failure is the only `error`
// a consumer surfaces.
export function useAssetFrames(id: string): AssetFramesState {
  const [frames, setFrames] = useState<SequenceFrame[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let stale = false;
    setLoading(true);
    setError(null);
    api
      .listAssetFrames(id)
      .then((f) => {
        if (stale) return;
        setFrames(f);
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

  return { frames, loading, error };
}
