import { useEffect, useRef, useState } from "react";
import type { Asset } from "../types";
import { fileUrl, thumbUrl, sequenceFrameUrl } from "../api/client";
import { useAssetFrames } from "../hooks/useAssetFrames";
import { IconChevronLeft, IconChevronRight } from "./icons";
import shell from "./AssetDetail.module.css";
import styles from "./SequenceViewer.module.css";

// Image sequence step viewer for the asset detail modal (issue #62). A run of
// consecutively-numbered images (explosion_0001.png ...) is indexed as ONE
// sequence asset; this fetches its frames and, when there are >= 2, shows a
// lightweight prev/next stepper with a frame counter (no autoplay, no thumbnail
// strip — the user asked for a light stepper). A plain image (0/1 frames)
// degrades to the same single-image preview as the ordinary image branch, so
// AssetMedia can mount this for every image asset without knowing the count.
//
// ←/→ page frames; like DocumentPager it owns a focused container and calls
// stopPropagation on those keys so paging never also switches the underlying
// asset via the browse layout's window listener. `go` refocuses the container
// after every change so focus cannot escape when a nav button disables at a
// boundary.
export function SequenceViewer({ asset }: { asset: Asset }) {
  const { frames, loading } = useAssetFrames(asset.id);
  const [cur, setCur] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const label = asset.display_name || asset.name;
  const multi = frames.length > 1;

  // Reset to the first frame whenever the asset changes (the hook refetches too).
  useEffect(() => {
    setCur(0);
  }, [asset.id]);

  // Grab focus so ←/→ land on this viewer (and get stopPropagation'd) rather
  // than the browse layout's window listener; only when the stepper is shown.
  useEffect(() => {
    if (multi) rootRef.current?.focus({ preventScroll: true });
  }, [multi]);

  // While loading, or for a plain image, show the ordinary single-image preview
  // (a link to the original) — identical to AssetMedia's image branch, so a
  // non-sequence image detail is visually unchanged.
  if (loading || !multi) {
    return (
      <a
        className={shell.imageLink}
        href={fileUrl(asset.id)}
        target="_blank"
        rel="noreferrer"
        title="查看原图"
      >
        <img className={shell.imagePreview} src={thumbUrl(asset.id)} alt={label} />
      </a>
    );
  }

  const go = (next: number) => {
    setCur(Math.min(frames.length - 1, Math.max(0, next)));
    rootRef.current?.focus({ preventScroll: true });
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    // Own ←/→: stop them reaching the browse layout's window listener.
    e.preventDefault();
    e.stopPropagation();
    go(e.key === "ArrowLeft" ? cur - 1 : cur + 1);
  };

  const frame = frames[cur];
  if (!frame) return null;

  return (
    // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
    <div ref={rootRef} className={styles.viewer} tabIndex={0} onKeyDown={onKeyDown} data-testid="sequence-viewer">
      <div className={styles.stage}>
        <img
          key={frame.frame_no}
          className={styles.frameImg}
          src={sequenceFrameUrl(asset.id, frame.frame_no)}
          alt={`${label} 帧 ${frame.frame_no}`}
          data-testid="sequence-frame-image"
        />
      </div>

      <div className={styles.controls}>
        <button
          type="button"
          className={styles.navBtn}
          title="上一帧 (←)"
          disabled={cur === 0}
          onClick={() => go(cur - 1)}
          data-testid="sequence-prev"
        >
          <IconChevronLeft width={22} height={22} />
        </button>
        <span className={styles.counter} data-testid="sequence-counter">
          {cur + 1} / {frames.length}
        </span>
        <button
          type="button"
          className={styles.navBtn}
          title="下一帧 (→)"
          disabled={cur === frames.length - 1}
          onClick={() => go(cur + 1)}
          data-testid="sequence-next"
        >
          <IconChevronRight width={22} height={22} />
        </button>
      </div>
    </div>
  );
}
