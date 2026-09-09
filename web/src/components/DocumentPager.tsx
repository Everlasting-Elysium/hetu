import { useEffect, useRef, useState } from "react";
import type { Asset } from "../types";
import { fileUrl, thumbUrl } from "../api/client";
import { useDocumentPages } from "../hooks/useDocumentPages";
import { IconChevronLeft, IconChevronRight, KindIcon } from "./icons";
import shell from "./AssetDetail.module.css";
import styles from "./DocumentPager.module.css";

// Multi-page document viewer for the asset detail modal (issue #48): a large
// current-page image above a clickable thumbnail strip, with prev/next buttons
// and ←/→ keyboard paging. A single-page (or thumbnail-less) document degrades
// to the same single-image / download preview as the image/default branches, so
// AssetMedia can mount this for every `kind === "document"` without knowing the
// page count.
//
// Keyboard conflict (real, deliberate): the browse layouts (grid/waterfall/
// gallery) each keep a window-level ←/→ listener mounted under this modal to move
// the asset cursor. This pager owns a focused container and calls
// stopPropagation on ←/→, which halts native bubbling at the React root (#root)
// before it reaches those window listeners — so paging never also switches the
// underlying asset. `go` refocuses the container after every change so focus can
// never escape to <body> (e.g. when a nav button disables at a boundary).
export function DocumentPager({ asset }: { asset: Asset }) {
  const { pages, loading, error } = useDocumentPages(asset.id);
  const [cur, setCur] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const activeThumbRef = useRef<HTMLButtonElement>(null);
  const label = asset.display_name || asset.name;
  const multi = pages.length > 1;

  // Reset to the first page whenever the asset changes (the hook refetches too).
  useEffect(() => {
    setCur(0);
  }, [asset.id]);

  // Grab focus so ←/→ land on this pager (and get stopPropagation'd) rather than
  // the browse layout's window listener; only when the paging UI is shown.
  useEffect(() => {
    if (multi) rootRef.current?.focus({ preventScroll: true });
  }, [multi]);

  // Keep the active thumbnail visible as the page changes.
  useEffect(() => {
    activeThumbRef.current?.scrollIntoView({ inline: "center", block: "nearest" });
  }, [cur]);

  if (loading) {
    return (
      <div className={shell.modelLoading}>
        <div className={shell.spinner} />
      </div>
    );
  }

  if (error) {
    return (
      <div className={shell.fallback} data-testid="pager-error">
        <KindIcon kind={asset.kind} width={72} height={72} />
        <p>页面加载失败：{error}</p>
        <a className="btn btn-primary" href={fileUrl(asset.id)} target="_blank" rel="noreferrer" download>
          下载文件
        </a>
      </div>
    );
  }

  // Single page (or none / no rendered thumbnails): mirror the image/default
  // preview — a thumbnail links to the original, or an icon + download when the
  // document has no rendered thumbnail at all.
  if (!multi) {
    return asset.thumb ? (
      <a className={shell.imageLink} href={fileUrl(asset.id)} target="_blank" rel="noreferrer" title="查看原文件">
        <img className={shell.imagePreview} src={thumbUrl(asset.id)} alt={label} />
      </a>
    ) : (
      <div className={shell.fallback}>
        <KindIcon kind={asset.kind} width={72} height={72} />
        <a className="btn btn-primary" href={fileUrl(asset.id)} target="_blank" rel="noreferrer" download>
          下载文件
        </a>
      </div>
    );
  }

  const go = (next: number) => {
    setCur(Math.min(pages.length - 1, Math.max(0, next)));
    rootRef.current?.focus({ preventScroll: true });
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    // Own ←/→: stop them reaching the browse layout's window listener.
    e.preventDefault();
    e.stopPropagation();
    go(e.key === "ArrowLeft" ? cur - 1 : cur + 1);
  };

  const page = pages[cur];
  if (!page) return null;

  return (
    // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
    <div ref={rootRef} className={styles.pager} tabIndex={0} onKeyDown={onKeyDown} data-testid="document-pager">
      <div className={styles.stage}>
        <img
          key={page.page_no}
          className={styles.pageImg}
          src={page.thumb_url}
          alt={`${label} 第 ${page.page_no} 页`}
          data-testid="pager-page-image"
        />
      </div>

      <div className={styles.controls}>
        <button
          type="button"
          className={styles.navBtn}
          title="上一页 (←)"
          disabled={cur === 0}
          onClick={() => go(cur - 1)}
          data-testid="pager-prev"
        >
          <IconChevronLeft width={22} height={22} />
        </button>
        <span className={styles.counter} data-testid="pager-counter">
          第 {cur + 1} / 共 {pages.length} 页
        </span>
        <button
          type="button"
          className={styles.navBtn}
          title="下一页 (→)"
          disabled={cur === pages.length - 1}
          onClick={() => go(cur + 1)}
          data-testid="pager-next"
        >
          <IconChevronRight width={22} height={22} />
        </button>
      </div>

      <div className={styles.strip}>
        {pages.map((p, i) => (
          <button
            key={p.page_no}
            ref={i === cur ? activeThumbRef : undefined}
            type="button"
            className={`${styles.thumb} ${i === cur ? styles.thumbActive : ""}`}
            title={`第 ${p.page_no} 页`}
            aria-current={i === cur}
            onClick={() => go(i)}
            data-testid="pager-thumb"
            data-page={p.page_no}
          >
            <img src={p.thumb_url} alt="" loading="lazy" decoding="async" />
          </button>
        ))}
      </div>
    </div>
  );
}
