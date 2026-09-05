import { useCallback, useRef, useState } from "react";

// Multi-select over an ordered id list. Supports plain click (replace),
// cmd/ctrl-click (toggle), and shift-click (contiguous range from anchor).
export interface Selection {
  selected: Set<string>;
  isSelected: (id: string) => boolean;
  count: number;
  toggle: (id: string) => void;
  select: (id: string, e: { shiftKey: boolean; metaKey: boolean; ctrlKey: boolean }) => void;
  selectAll: () => void;
  clear: () => void;
}

export function useSelection(ordered: string[]): Selection {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const anchor = useRef<string | null>(null);

  const isSelected = useCallback((id: string) => selected.has(id), [selected]);

  const toggle = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    anchor.current = id;
  }, []);

  const select = useCallback<Selection["select"]>(
    (id, e) => {
      if (e.shiftKey && anchor.current) {
        const from = ordered.indexOf(anchor.current);
        const to = ordered.indexOf(id);
        if (from !== -1 && to !== -1) {
          const [lo, hi] = from < to ? [from, to] : [to, from];
          setSelected(new Set(ordered.slice(lo, hi + 1)));
          return;
        }
      }
      if (e.metaKey || e.ctrlKey) {
        toggle(id);
        return;
      }
      setSelected(new Set([id]));
      anchor.current = id;
    },
    [ordered, toggle],
  );

  const selectAll = useCallback(() => setSelected(new Set(ordered)), [ordered]);
  const clear = useCallback(() => {
    setSelected(new Set());
    anchor.current = null;
  }, []);

  return { selected, isSelected, count: selected.size, toggle, select, selectAll, clear };
}
