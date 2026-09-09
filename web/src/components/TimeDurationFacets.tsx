import { DateRangeField } from "./DateRangeField";
import { RangeField } from "./RangeField";
import styles from "./FilterFacets.module.css";

// Seconds per minute — the scale that lets the 时长 range show minutes while the
// Query layer stores seconds (RangeField does the conversion at its boundary,
// like MB for file size).
const MINUTE = 60;

// Exported so FilterFacets/Sidebar forward this cohesive set as one object prop
// rather than nine flat ones — the whole group is only ever passed straight
// through to this component, and grouping keeps those files under the LOC ceiling.
export interface TimeDurationFacetsProps {
  minDuration: number; // seconds; 0 = no bound
  maxDuration: number;
  onSetDuration: (minDuration: number, maxDuration: number) => void;
  createdAfter: number; // unix seconds; 0 = no bound
  createdBefore: number;
  onSetCreatedRange: (createdAfter: number, createdBefore: number) => void;
  indexedAfter: number; // unix seconds; 0 = no bound
  indexedBefore: number;
  onSetIndexedRange: (indexedAfter: number, indexedBefore: number) => void;
}

// The 时长 (duration) + 创建时间 / 索引时间 (time) facets, split out of
// FilterFacets to keep each file under the LOC ceiling (issue #53). Duration is
// a minutes-displayed min–max range over the audio/video duration annotation
// (seconds committed); the two time rows are inclusive local-date ranges
// committed as unix seconds. Styled with the same section/RangeField rhythm as
// the shape/尺寸/文件大小 facets so they read as one filter language. Rendered by
// FilterFacets in both the main sidebar and the board asset panel.
export function TimeDurationFacets({
  minDuration,
  maxDuration,
  onSetDuration,
  createdAfter,
  createdBefore,
  onSetCreatedRange,
  indexedAfter,
  indexedBefore,
  onSetIndexedRange,
}: TimeDurationFacetsProps) {
  return (
    <>
      <div className={styles.section}>
        <div className={styles.head}>
          <span>时长</span>
          {(minDuration > 0 || maxDuration > 0) && (
            <button className={styles.clear} onClick={() => onSetDuration(0, 0)}>
              清除
            </button>
          )}
        </div>
        <RangeField
          label="时长"
          unit="分钟"
          min={minDuration}
          max={maxDuration}
          scale={MINUTE}
          step={0.1}
          onCommit={(mn, mx) => onSetDuration(mn, mx)}
        />
        <div className={styles.chips}>
          <button className={styles.chip} onClick={() => onSetDuration(0, MINUTE)}>
            &lt; 1分钟
          </button>
          <button className={styles.chip} onClick={() => onSetDuration(MINUTE, 5 * MINUTE)}>
            1–5分钟
          </button>
          <button className={styles.chip} onClick={() => onSetDuration(5 * MINUTE, 0)}>
            &gt; 5分钟
          </button>
        </div>
      </div>

      <div className={styles.section}>
        <div className={styles.head}>
          <span>创建时间</span>
          {(createdAfter > 0 || createdBefore > 0) && (
            <button className={styles.clear} onClick={() => onSetCreatedRange(0, 0)}>
              清除
            </button>
          )}
        </div>
        <DateRangeField
          label="创建时间"
          after={createdAfter}
          before={createdBefore}
          onCommit={onSetCreatedRange}
        />
      </div>

      <div className={styles.section}>
        <div className={styles.head}>
          <span>索引时间</span>
          {(indexedAfter > 0 || indexedBefore > 0) && (
            <button className={styles.clear} onClick={() => onSetIndexedRange(0, 0)}>
              清除
            </button>
          )}
        </div>
        <DateRangeField
          label="索引时间"
          after={indexedAfter}
          before={indexedBefore}
          onCommit={onSetIndexedRange}
        />
      </div>
    </>
  );
}
