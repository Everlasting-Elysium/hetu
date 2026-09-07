import { useCallback, useEffect, useReducer, useRef } from "react";
import { frameUrl, thumbUrl } from "../api/client";
import type { BoardItem } from "../types";

// Loads and caches board thumbnails as HTMLImageElement, the shape Konva's
// <Image image={...}> expects. Konva cannot consume a URL directly, so each
// image is decoded once via window.Image and reused across renders.
//
// Keyed by board-item id, not asset id: two items on one asset may pin different
// frames/angles (#86), so their images must cache and resolve apart. An item's
// source is, in priority: a client-captured override (object URL from the frame/
// angle picker) > a pinned video frame (frameUrl) > the asset's default thumb.
export interface BoardImages {
  get: (itemId: string) => HTMLImageElement | undefined;
}

const EMPTY: ReadonlyMap<string, string> = new Map();

function sourceUrl(item: BoardItem, overrides: ReadonlyMap<string, string>): string {
  const captured = overrides.get(item.id);
  if (captured) return captured;
  if (item.frame_ms != null) return frameUrl(item.asset_id, item.frame_ms);
  return thumbUrl(item.asset_id);
}

export function useBoardImages(
  items: BoardItem[],
  overrides: ReadonlyMap<string, string> = EMPTY,
): BoardImages {
  const cache = useRef<Map<string, HTMLImageElement>>(new Map());
  // itemId -> the URL its currently-decoded image was loaded from. Doubles as
  // the cache key and lets get() resolve synchronously during render.
  const keys = useRef<Map<string, string>>(new Map());
  const [, bump] = useReducer((n: number) => n + 1, 0);

  // Rebuild the index every render (cheap) so get() never lags a frame/angle
  // change; notes carry no asset and are skipped.
  keys.current = new Map();
  for (const item of items) {
    if (item.kind === "note") continue;
    keys.current.set(item.id, sourceUrl(item, overrides));
  }

  useEffect(() => {
    for (const url of keys.current.values()) {
      if (cache.current.has(url)) continue;
      const img = new window.Image();
      img.crossOrigin = "anonymous";
      img.onload = () => {
        cache.current.set(url, img);
        bump();
      };
      img.src = url;
    }
  }, [items, overrides]);

  // Reads the mutable cache; a load bump re-renders the host so callers pick up
  // freshly decoded images. Identity ignores the stable refs.
  return {
    get: useCallback((itemId: string) => {
      const url = keys.current.get(itemId);
      return url ? cache.current.get(url) : undefined;
    }, []),
  };
}
