// Konva paints to a raw <canvas>, which cannot resolve CSS custom properties.
// To keep variables.css the single source of truth, the canvas colors are read
// from the same design tokens at first use and cached. Fallbacks mirror the
// dark-theme values so a missing var never yields an empty fill string.
export interface BoardTheme {
  itemBg: string;
  accent: string;
  border: string;
  canvasBg: string;
  noteBg: string;
  noteText: string;
}

const FALLBACK: BoardTheme = {
  itemBg: "#191d23",
  accent: "#4f8ff7",
  border: "#363d47",
  canvasBg: "#0a0c0f",
  noteBg: "#fef3c7",
  noteText: "#78350f",
};

let cached: BoardTheme | null = null;

export function boardTheme(): BoardTheme {
  if (cached) return cached;
  const root = getComputedStyle(document.documentElement);
  const read = (name: string, fallback: string): string =>
    root.getPropertyValue(name).trim() || fallback;
  cached = {
    itemBg: read("--bg-card", FALLBACK.itemBg),
    accent: read("--accent", FALLBACK.accent),
    border: read("--border-strong", FALLBACK.border),
    canvasBg: read("--bg-inset", FALLBACK.canvasBg),
    noteBg: read("--note-bg", FALLBACK.noteBg),
    noteText: read("--note-text", FALLBACK.noteText),
  };
  return cached;
}
