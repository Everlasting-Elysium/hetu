import { useCallback, useEffect, useState } from "react";

export interface Fullscreen {
  isFullscreen: boolean;
  toggle: () => void;
}

// Drives the Fullscreen API for one element (the board `.view`). `isFullscreen`
// tracks the document via the native `fullscreenchange` event, so it also flips
// back when the user presses Esc — which exits fullscreen without our button.
export function useFullscreen(ref: React.RefObject<HTMLElement | null>): Fullscreen {
  const [isFullscreen, setIsFullscreen] = useState(false);

  useEffect(() => {
    const onChange = () => setIsFullscreen(document.fullscreenElement === ref.current);
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, [ref]);

  const toggle = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    if (document.fullscreenElement) {
      void document.exitFullscreen();
    } else {
      void el.requestFullscreen();
    }
  }, [ref]);

  return { isFullscreen, toggle };
}
