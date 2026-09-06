import { useEffect, useState } from "react";
import { BROWSE_LAYOUTS, isBrowseLayout, type ViewMode } from "../types";

const KEY = "hetu.viewMode";

// Restores the last browse layout (grid/waterfall/gallery) from localStorage.
// Trash/missing/immersive are transient and never used as the startup view.
function loadInitial(): ViewMode {
  try {
    const stored = localStorage.getItem(KEY);
    if (stored && (BROWSE_LAYOUTS as readonly string[]).includes(stored)) {
      return stored as ViewMode;
    }
  } catch {
    /* storage unavailable (private mode / disabled) */
  }
  return "grid";
}

// Owns the active view + persistence. Only browse layouts are written back, so a
// reload lands on the user's preferred grid/waterfall/gallery, never in an overlay.
export function useViewMode() {
  const [view, setView] = useState<ViewMode>(loadInitial);

  useEffect(() => {
    if (!isBrowseLayout(view)) return;
    try {
      localStorage.setItem(KEY, view);
    } catch {
      /* ignore write failures */
    }
  }, [view]);

  return [view, setView] as const;
}
