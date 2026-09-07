import { useCallback, useEffect, useRef, useState } from "react";

// Owns the per-item captured-thumbnail object URLs produced by the video-frame
// and model-angle pickers (#86): each capture mints a fresh object URL, revokes
// the previous one for that item, and everything is revoked on unmount so a long
// editing session never leaks blobs. BoardCanvas passes `urls` to useBoardImages
// as the image override for a pinned frame/angle — the freshly captured still
// shows instantly, before (video) or in place of (model) any server render.
export interface BoardCaptures {
  urls: ReadonlyMap<string, string>;
  set: (itemId: string, blob: Blob) => void;
}

export function useBoardCaptures(): BoardCaptures {
  const [urls, setUrls] = useState<Map<string, string>>(new Map());
  const ref = useRef(urls);
  ref.current = urls;

  useEffect(
    () => () => {
      for (const url of ref.current.values()) URL.revokeObjectURL(url);
    },
    [],
  );

  const set = useCallback((itemId: string, blob: Blob) => {
    const url = URL.createObjectURL(blob);
    setUrls((prev) => {
      const next = new Map(prev);
      const old = next.get(itemId);
      if (old) URL.revokeObjectURL(old);
      next.set(itemId, url);
      return next;
    });
  }, []);

  return { urls, set };
}
