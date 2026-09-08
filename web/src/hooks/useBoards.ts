import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { Board, BoardItem } from "../types";

const errMsg = (e: unknown): string => (e instanceof Error ? e.message : String(e));

// Batch-save debounce after canvas edits (drag/resize/rotate/z). Kept small so
// the network settles quickly once the user pauses interacting.
const SAVE_DEBOUNCE_MS = 800;

export interface Boards {
  list: Board[];
  reload: () => void;
  createBoard: (name: string) => Promise<Board | null>;
  renameBoard: (id: string, name: string) => Promise<void>;
  deleteBoard: (id: string) => Promise<void>;
}

// Owns the moodboard list + its mutations, mirroring useLibrary so App stays a
// thin composer. Single-board editing lives in useBoard below.
export function useBoards(onError: (msg: string) => void): Boards {
  const [list, setList] = useState<Board[]>([]);

  const guard = useCallback(
    async (fn: () => Promise<void>) => {
      try {
        await fn();
      } catch (e) {
        onError(errMsg(e));
      }
    },
    [onError],
  );

  const reload = useCallback(() => guard(async () => setList(await api.listBoards())), [guard]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const createBoard = useCallback(
    async (name: string): Promise<Board | null> => {
      try {
        const created = await api.createBoard(name);
        await reload();
        return created;
      } catch (e) {
        onError(errMsg(e));
        return null;
      }
    },
    [reload, onError],
  );

  return {
    list,
    reload,
    createBoard,
    renameBoard: (id, name) => guard(async () => { await api.renameBoard(id, name); await reload(); }),
    deleteBoard: (id) => guard(async () => { await api.deleteBoard(id); await reload(); }),
  };
}

// The canvas box for a newly dropped item, reusing BoardItem's own geometry
// fields so the two never drift.
export type ItemBox = Pick<BoardItem, "x" | "y" | "w" | "h">;

export interface ActiveBoard {
  board: Board | null;
  items: BoardItem[];
  addItem: (assetId: string, box: ItemBox) => Promise<void>;
  addNote: (text: string, box: ItemBox) => Promise<BoardItem | null>;
  updateItems: (next: BoardItem[]) => void;
  patchItem: (id: string, patch: Partial<BoardItem>) => void;
  removeItem: (itemId: string) => Promise<void>;
  duplicateItems: (sources: BoardItem[]) => Promise<string[]>;
}

// Loads one board with its items and manages canvas edits. `updateItems` sets
// local state immediately (so the canvas stays responsive) and debounces a
// single PATCH covering every item; `addItem`/`removeItem` persist eagerly.
export function useBoard(boardId: string | null, onError: (msg: string) => void): ActiveBoard {
  const [board, setBoard] = useState<Board | null>(null);
  const [items, setItems] = useState<BoardItem[]>([]);
  const itemsRef = useRef<BoardItem[]>([]);
  const timer = useRef<number | null>(null);
  const pending = useRef<BoardItem[] | null>(null);
  const boardIdRef = useRef(boardId);
  itemsRef.current = items;
  boardIdRef.current = boardId;

  useEffect(() => {
    if (!boardId) {
      setBoard(null);
      setItems([]);
      return;
    }
    let alive = true;
    api
      .getBoard(boardId)
      .then((b) => {
        if (!alive) return;
        setBoard(b);
        setItems(b.items ?? []);
      })
      .catch((e: unknown) => {
        if (alive) onError(errMsg(e));
      });
    return () => {
      alive = false;
    };
  }, [boardId, onError]);

  // Flush a still-pending debounced save on unmount so navigating away within
  // the debounce window never drops the last edit.
  useEffect(
    () => () => {
      if (timer.current === null) return;
      window.clearTimeout(timer.current);
      const bid = boardIdRef.current;
      if (bid && pending.current) void api.updateBoardItems(bid, pending.current).catch(() => {});
    },
    [],
  );

  const scheduleSave = useCallback(
    (next: BoardItem[]) => {
      if (!boardId) return;
      pending.current = next;
      if (timer.current !== null) window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => {
        pending.current = null;
        api.updateBoardItems(boardId, next).catch((e: unknown) => onError(errMsg(e)));
      }, SAVE_DEBOUNCE_MS);
    },
    [boardId, onError],
  );

  const updateItems = useCallback(
    (next: BoardItem[]) => {
      setItems(next);
      scheduleSave(next);
    },
    [scheduleSave],
  );

  // Merge a partial patch into one item by id, using a functional updater so
  // concurrent Konva transform callbacks (one per selected node, fired
  // synchronously) each read the latest state instead of a stale closure.
  const patchItem = useCallback(
    (id: string, patch: Partial<BoardItem>) => {
      setItems((prev) => {
        const next = prev.map((it) => (it.id === id ? { ...it, ...patch } : it));
        scheduleSave(next);
        return next;
      });
    },
    [scheduleSave],
  );

  const addItem = useCallback(
    async (assetId: string, box: ItemBox) => {
      if (!boardId) return;
      const z = itemsRef.current.reduce((m, it) => Math.max(m, it.z), 0) + 1;
      try {
        const created = await api.addBoardItem(boardId, {
          asset_id: assetId,
          ...box,
          rotation: 0,
          z,
        });
        setItems((prev) => [...prev, created]);
      } catch (e) {
        onError(errMsg(e));
      }
    },
    [boardId, onError],
  );

  // A note is persisted eagerly like addItem, but returns the created item so
  // the canvas can drop the caret straight into it for editing.
  const addNote = useCallback(
    async (text: string, box: ItemBox): Promise<BoardItem | null> => {
      if (!boardId) return null;
      try {
        const created = await api.addBoardNote(boardId, { text, ...box });
        setItems((prev) => [...prev, created]);
        return created;
      } catch (e) {
        onError(errMsg(e));
        return null;
      }
    },
    [boardId, onError],
  );

  const removeItem = useCallback(
    async (itemId: string) => {
      if (!boardId) return;
      try {
        await api.deleteBoardItem(boardId, itemId);
        setItems((prev) => prev.filter((it) => it.id !== itemId));
      } catch (e) {
        onError(errMsg(e));
      }
    },
    [boardId, onError],
  );

  // Duplicate a set of items — each copy gets a new server-generated id, an
  // offset position, and *independent* frame_ms / view so video/model copies
  // can be re-edited without affecting the original.  Returns the new ids so
  // the caller can auto-select the copies.
  const DUP_OFFSET = 20;
  const duplicateItems = useCallback(
    async (sources: BoardItem[]): Promise<string[]> => {
      if (!boardId || sources.length === 0) return [];
      try {
        const created: BoardItem[] = [];
        for (const src of sources) {
          const maxZ = [...itemsRef.current, ...created].reduce(
            (m, it) => Math.max(m, it.z),
            0,
          );
          const item = await api.addBoardItem(boardId, {
            kind: src.kind ?? "asset",
            asset_id: src.kind === "note" ? "" : src.asset_id,
            x: src.x + DUP_OFFSET,
            y: src.y + DUP_OFFSET,
            w: src.w,
            h: src.h,
            rotation: src.rotation,
            z: maxZ + 1,
            // exactOptionalPropertyTypes: omit optional fields rather than pass
            // an explicit `undefined` (which breaks the strict tsc build).
            ...(src.kind === "note" ? { text: src.text ?? "" } : {}),
            ...(src.frame_ms !== undefined ? { frame_ms: src.frame_ms } : {}),
            ...(src.view !== undefined ? { view: src.view } : {}),
          });
          created.push(item);
        }
        setItems((prev) => [...prev, ...created]);
        return created.map((it) => it.id);
      } catch (e) {
        onError(errMsg(e));
        return [];
      }
    },
    [boardId, onError],
  );

  return { board, items, addItem, addNote, updateItems, patchItem, removeItem, duplicateItems };
}
