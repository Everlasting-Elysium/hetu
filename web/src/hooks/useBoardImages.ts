import { useCallback, useEffect, useReducer, useRef } from "react";
import { thumbUrl } from "../api/client";

// Loads and caches board thumbnails as HTMLImageElement, the shape Konva's
// <Image image={...}> expects. Konva cannot consume a URL directly, so each
// asset's thumb is decoded once via window.Image and reused across renders.
export interface BoardImages {
  get: (assetId: string) => HTMLImageElement | undefined;
}

export function useBoardImages(assetIds: string[]): BoardImages {
  const cache = useRef<Map<string, HTMLImageElement>>(new Map());
  const [, bump] = useReducer((n: number) => n + 1, 0);

  useEffect(() => {
    for (const id of assetIds) {
      if (cache.current.has(id)) continue;
      const img = new window.Image();
      img.crossOrigin = "anonymous";
      img.onload = () => {
        cache.current.set(id, img);
        bump();
      };
      img.src = thumbUrl(id);
    }
  }, [assetIds]);

  // Reads the mutable cache; a load bump re-renders the host so callers pick up
  // freshly decoded images. Identity ignores `cache` (a stable ref).
  return { get: useCallback((id: string) => cache.current.get(id), []) };
}
