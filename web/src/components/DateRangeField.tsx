import styles from "./FilterFacets.module.css";

// Date <-> unix-second conversions for the time-range inputs. They live only in
// this component so the Query layer holds unix seconds, never date strings or
// Date objects (issue #53) — the same boundary discipline RangeField keeps for
// MB/bytes. A day picked as the "after" bound means its local start (00:00:00);
// as the "before" bound it means its local end (23:59:59), so a single day
// selected on both sides spans that whole day inclusively.
const pad = (n: number): string => String(n).padStart(2, "0");

const unixToDate = (sec: number): string => {
  if (!sec) return "";
  const d = new Date(sec * 1000);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

// A date-time string without a timezone ("YYYY-MM-DDT..") is parsed as LOCAL
// time per the ES spec (unlike a bare "YYYY-MM-DD", which is UTC), so appending
// the local wall-clock start/end of day yields the intended local-day bounds and
// round-trips through unixToDate above. A native date input only ever emits a
// complete date or "", so no partial-string guard beyond the empty check.
const dayStartUnix = (dateStr: string): number =>
  dateStr ? Math.floor(new Date(`${dateStr}T00:00:00`).getTime() / 1000) : 0;

const dayEndUnix = (dateStr: string): number =>
  dateStr ? Math.floor(new Date(`${dateStr}T23:59:59`).getTime() / 1000) : 0;

interface Props {
  label: string;
  after: number; // unix seconds; 0 = no bound
  before: number; // unix seconds; 0 = no bound
  onCommit: (after: number, before: number) => void;
}

// A labeled start–end date range. Values in/out are unix seconds; the date
// pickers show/parse local dates. It is fully controlled from props (a native
// date input only fires onChange with a complete or cleared value, so there is
// no in-progress partial to protect the way RangeField's decimal text needs).
export function DateRangeField({ label, after, before, onCommit }: Props) {
  return (
    <div className={styles.rangeRow}>
      <span className={styles.rangeLabel}>{label}</span>
      <input
        className={`input ${styles.rangeInput}`}
        type="date"
        aria-label={`起始${label}`}
        value={unixToDate(after)}
        onChange={(e) => onCommit(dayStartUnix(e.target.value), before)}
      />
      <span className={styles.rangeSep}>–</span>
      <input
        className={`input ${styles.rangeInput}`}
        type="date"
        aria-label={`结束${label}`}
        value={unixToDate(before)}
        onChange={(e) => onCommit(after, dayEndUnix(e.target.value))}
      />
    </div>
  );
}
