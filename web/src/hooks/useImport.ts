import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";

// Context the import entry points read live at event time. Kept in a ref (below)
// so the once-installed window paste listener and the async import loop never
// close over a stale folder, enabled flag, or callback.
export interface ImportCtx {
  folderId: string | null; // non-null → move imports into this virtual folder (#89)
  enabled: boolean; // only the browse layouts (grid/waterfall/gallery) accept imports
  onDone: () => void; // bump() — refetch the asset list after a batch
  onNotice: (msg: string) => void; // success toast
  onError: (msg: string) => void; // failure toast
}

export interface UseImport {
  dragging: boolean;
  dropHandlers: {
    onDragEnter: (e: React.DragEvent) => void;
    onDragOver: (e: React.DragEvent) => void;
    onDragLeave: (e: React.DragEvent) => void;
    onDrop: (e: React.DragEvent) => void;
  };
}

// Synthesizes a filename for a clipboard/drag blob so the backend infers kind/ext
// from the extension. A real name that already has an extension is kept; a
// nameless paste blob becomes pasted-<ts>-<i>.<ext> (the index disambiguates a
// multi-item paste landing in the same millisecond). Falls back to the MIME
// subtype, then bin, when the type is unknown.
const MIME_EXT: Record<string, string> = {
  "image/png": "png",
  "image/jpeg": "jpg",
  "image/gif": "gif",
  "image/webp": "webp",
  "image/bmp": "bmp",
  "image/svg+xml": "svg",
  "image/avif": "avif",
  "image/tiff": "tiff",
};

function fileName(f: File, i: number): string {
  if (f.name && /\.[a-z0-9]+$/i.test(f.name)) return f.name;
  const ext = MIME_EXT[f.type] ?? f.type.split("/")[1] ?? "bin";
  return `pasted-${Date.now()}-${i}.${ext}`;
}

// Collects the file-kind items from a clipboard/drag transfer, ignoring string
// items (plain-text paste). DataTransferItemList is indexable but not iterable
// in the TS lib types, so index by hand.
function filesFrom(dt: DataTransfer | null): File[] {
  if (!dt) return [];
  const out: File[] = [];
  for (let i = 0; i < dt.items.length; i++) {
    const it = dt.items[i];
    if (it?.kind === "file") {
      const f = it.getAsFile();
      if (f) out.push(f);
    }
  }
  return out;
}

const isEditable = (el: Element | null): boolean =>
  el instanceof HTMLInputElement ||
  el instanceof HTMLTextAreaElement ||
  (el instanceof HTMLElement && el.isContentEditable);

const hasFiles = (e: React.DragEvent): boolean => e.dataTransfer.types.includes("Files");

// useImport wires the two "zero-leave" import entry points for the asset page
// (issue #89): external file drag-drop onto the grid, and Ctrl/⌘+V paste. Both
// funnel into one importFiles() — upload each file (copy+index+thumbnail+dedupe
// server-side), optionally move it into the active folder, then refresh once and
// report an aggregate "已导入 N / 共 M(跳过 K)". A single item's failure never
// aborts the rest.
export function useImport(ctx: ImportCtx): UseImport {
  const [dragging, setDragging] = useState(false);
  // dragenter/leave fire per child element; a depth counter clears the overlay
  // only when the cursor actually leaves the container, not on inner crossings.
  const depth = useRef(0);
  const ctxRef = useRef(ctx);
  ctxRef.current = ctx;

  const importFiles = useCallback(async (files: File[]) => {
    const c = ctxRef.current;
    if (files.length === 0) return;
    let done = 0;
    let skipped = 0;
    const failures: string[] = [];
    // Sequential (concurrency 1): the import endpoint is synchronous and single
    // file, so serializing keeps a large paste/drop from stampeding it.
    for (let i = 0; i < files.length; i++) {
      const f = files[i];
      if (!f) continue;
      try {
        const res = await api.importAsset(f, fileName(f, i));
        if (res.skipped) {
          skipped++;
          continue;
        }
        done++;
        if (c.folderId && res.asset) await api.move([res.asset.id], c.folderId);
      } catch (e) {
        failures.push(e instanceof Error ? e.message : String(e));
      }
    }
    c.onDone();
    if (done > 0 || skipped > 0) {
      c.onNotice(`已导入 ${done} / 共 ${files.length}${skipped > 0 ? `(跳过 ${skipped})` : ""}`);
    }
    if (failures.length > 0) {
      c.onError(`${failures.length} 个导入失败:${failures[0]}`);
    }
  }, []);

  // App-level paste: installed once, reads live context via the ref. Skips when
  // an input/editable is focused (let the field paste text) or when the
  // clipboard holds no files (plain-text paste is not ours to hijack).
  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      if (!ctxRef.current.enabled || isEditable(document.activeElement)) return;
      const files = filesFrom(e.clipboardData);
      if (files.length === 0) return;
      e.preventDefault();
      void importFiles(files);
    };
    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  }, [importFiles]);

  // Drag handlers take over ONLY for external file drags (dataTransfer has
  // "Files"), so internal board/asset-id drags pass through untouched.
  const onDragEnter = useCallback((e: React.DragEvent) => {
    if (!ctxRef.current.enabled || !hasFiles(e)) return;
    e.preventDefault();
    depth.current += 1;
    setDragging(true);
  }, []);

  const onDragOver = useCallback((e: React.DragEvent) => {
    if (!ctxRef.current.enabled || !hasFiles(e)) return;
    e.preventDefault(); // required so the drop event fires
  }, []);

  const onDragLeave = useCallback(() => {
    if (!ctxRef.current.enabled || depth.current === 0) return;
    depth.current -= 1;
    if (depth.current === 0) setDragging(false);
  }, []);

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      if (!ctxRef.current.enabled || !hasFiles(e)) return;
      e.preventDefault();
      depth.current = 0;
      setDragging(false);
      void importFiles(Array.from(e.dataTransfer.files));
    },
    [importFiles],
  );

  return { dragging, dropHandlers: { onDragEnter, onDragOver, onDragLeave, onDrop } };
}
