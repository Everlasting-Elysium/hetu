import { useCallback, useEffect, useState } from "react";
import type Konva from "konva";
import type { BoardItem } from "../types";

// Modifier flags read off a click; any of them turns a plain (replace) click
// into a toggle that grows/shrinks the current multi-selection.
export interface SelectMods {
  metaKey?: boolean;
  ctrlKey?: boolean;
  shiftKey?: boolean;
}

export interface BoardSelection {
  selectedIds: Set<string>;
  selectItem: (id: string, mods?: SelectMods) => void;
  clear: () => void;
}

interface Params {
  items: BoardItem[];
  patchItem: (id: string, patch: Partial<BoardItem>) => void;
  removeItem: (id: string) => Promise<void>;
  stageRef: React.RefObject<Konva.Stage | null>;
  trRef: React.RefObject<Konva.Transformer | null>;
}

// Owns the board's multi-selection: which item ids are selected, binding the
// shared Transformer to every selected Konva node, and deleting the whole
// selection on Delete/Backspace. Kept out of BoardCanvas so the canvas stays a
// thin composer and the selection rules live in one testable place.
export function useBoardSelection(p: Params): BoardSelection {
  const { items, patchItem, removeItem, stageRef, trRef } = p;
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  // Re-bind the Transformer whenever the selection or the items re-render, so a
  // resize/rotate handle wraps every selected node (multi-node transform).
  useEffect(() => {
    const tr = trRef.current;
    const stage = stageRef.current;
    if (!tr || !stage) return;
    const nodes = [...selectedIds]
      .map((id) => stage.findOne(`#${id}`))
      .filter((n): n is Konva.Node => !!n);
    tr.nodes(nodes);
    tr.getLayer()?.batchDraw();
  }, [selectedIds, items, stageRef, trRef]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Delete" && e.key !== "Backspace") return;
      if (selectedIds.size === 0) return;
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;
      for (const id of selectedIds) void removeItem(id);
      setSelectedIds(new Set());
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selectedIds, removeItem]);

  const selectItem = useCallback(
    (id: string, mods?: SelectMods) => {
      const multi = Boolean(mods?.metaKey || mods?.ctrlKey || mods?.shiftKey);
      setSelectedIds((prev) => {
        const next = new Set(multi ? prev : []);
        if (multi && prev.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
      // Only bump z on a plain (non-modifier) select so building a
      // multi-selection does not cause extra dirty writes and z reordering.
      if (!multi) {
        const maxZ = items.reduce((m, it) => Math.max(m, it.z), 0);
        const target = items.find((it) => it.id === id);
        if (target && target.z < maxZ) patchItem(id, { z: maxZ + 1 });
      }
    },
    [items, patchItem],
  );

  const clear = useCallback(() => setSelectedIds(new Set()), []);

  return { selectedIds, selectItem, clear };
}
