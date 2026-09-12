import { useCallback, useRef, useState } from "react";
import { api, thumbUrl } from "../api/client";
import type { Asset, CompareInput, CompareResponse, CompareSide } from "../types";

// Which comparison side a slot fills.
export type Side = "reference" | "target";

// A filled comparison slot: a library asset (thumb + name for display) or a
// transient local file (object URL for preview). The file variant is uploaded
// straight to /compare and is NEVER imported into the library (issue #127).
export type CompareSlot =
  | { kind: "asset"; assetId: string; name: string; thumb: string }
  | { kind: "file"; file: File; url: string };

export interface UseCompare {
  reference: CompareSlot | null;
  target: CompareSlot | null;
  result: CompareResponse | null;
  loading: boolean;
  // Seed both slots from two selected library assets (the batch-bar entry point).
  seed: (ref: Asset, tgt: Asset) => void;
  // Replace one slot with a transient local file (drag/drop or file picker).
  setFile: (side: Side, file: File) => void;
  // Exchange reference and target.
  swap: () => void;
  // Run the comparison; reports failures through onError, never throws.
  run: (onError: (msg: string) => void) => Promise<void>;
  // Drop all slots + result and release any object URLs (on leaving the page).
  reset: () => void;
}

const assetSlot = (a: Asset): CompareSlot => ({
  kind: "asset",
  assetId: a.id,
  name: a.display_name || a.name,
  thumb: thumbUrl(a.id),
});

const sideInput = (slot: CompareSlot): CompareSide =>
  slot.kind === "asset"
    ? { kind: "asset", assetId: slot.assetId }
    : { kind: "file", file: slot.file };

// Release a file slot's object URL so replacing/swapping/leaving never leaks.
const revoke = (slot: CompareSlot | null) => {
  if (slot?.kind === "file") URL.revokeObjectURL(slot.url);
};

// Owns the two-image comparison state: the reference/target slots, the last
// result, and the in-flight flag. Both slots live in one object so `swap` is a
// single atomic update; a ref mirrors them so `run` reads the latest without a
// stale closure. Any slot change clears the previous result.
export function useCompare(): UseCompare {
  const [slots, setSlots] = useState<{ reference: CompareSlot | null; target: CompareSlot | null }>(
    { reference: null, target: null },
  );
  const [result, setResult] = useState<CompareResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const slotsRef = useRef(slots);
  slotsRef.current = slots;

  const seed = useCallback((ref: Asset, tgt: Asset) => {
    setSlots((s) => {
      revoke(s.reference);
      revoke(s.target);
      return { reference: assetSlot(ref), target: assetSlot(tgt) };
    });
    setResult(null);
  }, []);

  const setFile = useCallback((side: Side, file: File) => {
    const slot: CompareSlot = { kind: "file", file, url: URL.createObjectURL(file) };
    setSlots((s) => {
      revoke(s[side]);
      return { ...s, [side]: slot };
    });
    setResult(null);
  }, []);

  const swap = useCallback(() => {
    setSlots((s) => ({ reference: s.target, target: s.reference }));
    setResult(null);
  }, []);

  const run = useCallback(async (onError: (msg: string) => void) => {
    const { reference, target } = slotsRef.current;
    if (!reference || !target) return;
    const input: CompareInput = { reference: sideInput(reference), target: sideInput(target) };
    setLoading(true);
    try {
      setResult(await api.compare(input));
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  const reset = useCallback(() => {
    setSlots((s) => {
      revoke(s.reference);
      revoke(s.target);
      return { reference: null, target: null };
    });
    setResult(null);
    setLoading(false);
  }, []);

  return { reference: slots.reference, target: slots.target, result, loading, seed, setFile, swap, run, reset };
}
