import { useEffect, useRef, useState } from "react";
import styles from "./FilterFacets.module.css";

// One megabyte in bytes — the scale used when a range shows MB but stores bytes.
export const MB = 1024 * 1024;

// Matches the keyword search box's existing debounce (SearchBar/BoardAssetPanel
// both use a 300ms setTimeout) so range typing follows the same convention: the
// network-triggering commit waits for a pause in typing, while the visible text
// below stays instant on every keystroke.
const COMMIT_DEBOUNCE_MS = 300;

// Text <-> canonical conversions for the range inputs. scale=1 keeps pixels
// as-is; scale=MB turns the MB a user types into the bytes the query layer
// stores, so the MB/byte conversion lives only at this input boundary (#101).
const toCanonical = (text: string, scale: number): number =>
  Math.max(0, Math.round((Number(text) || 0) * scale));
const toText = (value: number, scale: number): string => (value ? String(value / scale) : "");

interface Props {
  label: string;
  unit: string;
  min: number;
  max: number;
  onCommit: (min: number, max: number) => void;
  // scale converts input units to the canonical value (1 for pixels, MB for
  // file size); step < 1 also switches the field to a decimal keypad.
  scale?: number;
  step?: number;
}

// A labeled min–max numeric range (宽/高/大小). Values in/out are canonical
// (pixels, or bytes when scale=MB). Local text state is load-bearing: a fully
// controlled MB field would round-trip every keystroke through bytes and wipe an
// in-progress decimal ("0." -> 0 -> ""), so the field owns its text and only
// re-syncs from the canonical value when a parent clear/preset changes it to
// something the field did not just commit (#101).
export function RangeField({ label, unit, min, max, onCommit, scale = 1, step = 1 }: Props) {
  const [lo, setLo] = useState(() => toText(min, scale));
  const [hi, setHi] = useState(() => toText(max, scale));
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    setLo((prev) => (toCanonical(prev, scale) === min ? prev : toText(min, scale)));
  }, [min, scale]);
  useEffect(() => {
    setHi((prev) => (toCanonical(prev, scale) === max ? prev : toText(max, scale)));
  }, [max, scale]);
  // A debounced commit must not fire after the field is gone (e.g. the user
  // navigated away mid-pause).
  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  // Debounces onCommit (the call that reaches setQuery -> useAssets/useFacets and
  // triggers an HTTP request) so typing a multi-digit number fires one request,
  // not one per keystroke. Reads BOTH text fields fresh at schedule time (not a
  // value threaded in from a stale render) so alternating edits between min and
  // max within the debounce window commit the latest of each, never drop one.
  const scheduleCommit = (nextLo: string, nextHi: string) => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(
      () => onCommit(toCanonical(nextLo, scale), toCanonical(nextHi, scale)),
      COMMIT_DEBOUNCE_MS,
    );
  };
  const mode = step < 1 ? "decimal" : "numeric";

  return (
    <div className={styles.rangeRow}>
      <span className={styles.rangeLabel}>{label}</span>
      <input
        className={`input ${styles.rangeInput}`}
        type="number"
        min={0}
        step={step}
        inputMode={mode}
        placeholder="最小"
        aria-label={`最小${label}（${unit}）`}
        value={lo}
        onChange={(e) => {
          setLo(e.target.value);
          scheduleCommit(e.target.value, hi);
        }}
      />
      <span className={styles.rangeSep}>–</span>
      <input
        className={`input ${styles.rangeInput}`}
        type="number"
        min={0}
        step={step}
        inputMode={mode}
        placeholder="最大"
        aria-label={`最大${label}（${unit}）`}
        value={hi}
        onChange={(e) => {
          setHi(e.target.value);
          scheduleCommit(lo, e.target.value);
        }}
      />
      <span className={styles.rangeUnit}>{unit}</span>
    </div>
  );
}
