import { useEffect, useRef } from "react";
import type { BoardItem } from "../types";
import type { AlignOp } from "../components/boardAlign";

interface Params {
  selectedItems: BoardItem[];
  onApply: (op: AlignOp) => void;
}

// Resolve a keydown to an arrange op, mirroring PureRef's shortcut scheme with
// Ctrl/Cmd as the modifier. Plain Mod+Arrow aligns to an edge; +Alt normalizes
// size (aspect-preserving); +Alt+Shift distributes. Returns null when the combo
// is not an arrange shortcut so the caller leaves the event untouched.
function opFor(e: KeyboardEvent): AlignOp | null {
  if (!(e.ctrlKey || e.metaKey)) return null;
  const k = e.key;
  if (e.altKey && e.shiftKey) {
    if (k === "ArrowUp") return "distributeH";
    if (k === "ArrowDown") return "distributeV";
    return null;
  }
  if (e.altKey) {
    if (k === "ArrowRight") return "matchW";
    if (k === "ArrowLeft") return "matchH";
    if (k === "ArrowUp") return "matchSize";
    return null;
  }
  if (k === "ArrowLeft") return "left";
  if (k === "ArrowRight") return "right";
  if (k === "ArrowUp") return "top";
  if (k === "ArrowDown") return "bottom";
  return null;
}

// Keyboard arranging for the board, matching PureRef: with two or more items
// selected, Ctrl/Cmd+Arrow aligns, +Alt normalizes size (aspect-preserving
// equal width / height / longest-side), +Alt+Shift distributes. Reads the live
// selection + handler through refs so the once-attached listener never fires on
// a stale closure. Ignores keystrokes while typing in an input/textarea.
export function useArrangeShortcuts({ selectedItems, onApply }: Params): void {
  const itemsRef = useRef(selectedItems);
  const applyRef = useRef(onApply);
  itemsRef.current = selectedItems;
  applyRef.current = onApply;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (itemsRef.current.length < 2) return;
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;
      const op = opFor(e);
      if (!op) return;
      e.preventDefault();
      applyRef.current(op);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
}
