// Clipboard helpers for copying selected assets to the system clipboard.
//
// Browser Clipboard API constraints (annotated inline):
//   - navigator.clipboard.write requires a secure context (https or localhost).
//   - ClipboardItem only reliably supports "image/png" for bitmap data across
//     browsers; non-PNG images (jpg/webp/svg/gif) are converted via OffscreenCanvas.
//   - GIF → PNG loses animation (only first frame); this is a known trade-off
//     because the Clipboard API has no animated-image MIME support.
//   - Chrome accepts Promise<Blob> inside ClipboardItem, preserving the user-gesture
//     context across async fetch+convert. Safari 16.4+ also supports this form.
//     We use the Promise<Blob> approach as it is the most broadly compatible path
//     that avoids losing the gesture; if a browser rejects it, the catch path
//     surfaces a clear error toast rather than silently failing.

import type { Asset } from "../types";
import { fileUrl } from "../api/client";

/** Convert any image blob to PNG via OffscreenCanvas. */
async function toPngBlob(blob: Blob): Promise<Blob> {
  const bmp = await createImageBitmap(blob);
  const canvas = new OffscreenCanvas(bmp.width, bmp.height);
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("OffscreenCanvas 2d context unavailable");
  ctx.drawImage(bmp, 0, 0);
  bmp.close();
  return canvas.convertToBlob({ type: "image/png" });
}

/** Fetch the original file and return it as a PNG blob. */
async function fetchAsPng(assetId: string): Promise<Blob> {
  const res = await fetch(fileUrl(assetId));
  if (!res.ok) throw new Error(`fetch failed: ${res.status}`);
  const blob = await res.blob();
  if (blob.type === "image/png") return blob;
  return toPngBlob(blob);
}

/** Build the absolute URL for an asset's original file. */
function absoluteFileUrl(assetId: string): string {
  return location.origin + fileUrl(assetId);
}

/**
 * Resolve which assets to copy and in what form.
 *
 * - sel non-empty → use sel as target set
 * - else focusedId → single-item target
 * - both empty → null (no-op)
 */
function resolveTargets(
  assets: Asset[],
  selected: Set<string>,
  focusedId: string | null,
): Asset[] | null {
  if (selected.size > 0) {
    // Preserve the order they appear in the asset list.
    const targets = assets.filter((a) => selected.has(a.id));
    return targets.length > 0 ? targets : null;
  }
  if (focusedId) {
    const focused = assets.find((a) => a.id === focusedId);
    return focused ? [focused] : null;
  }
  return null;
}

/** Pick the primary image for the bitmap slot: focusedId if image, else first image. */
function pickPrimaryImage(targets: Asset[], focusedId: string | null): Asset | null {
  if (focusedId) {
    const focused = targets.find((a) => a.id === focusedId && a.kind === "image");
    if (focused) return focused;
  }
  return targets.find((a) => a.kind === "image") ?? null;
}

export interface CopyResult {
  /** "image" if a bitmap was written, "url" if only text URLs. */
  type: "image" | "url";
  count: number;
}

/**
 * Copy the resolved asset set to the system clipboard.
 *
 * Design decisions (from issue #90):
 *   - Multi-select: clipboard holds ONE bitmap (primary image) + text/plain with
 *     ALL selected items' absolute URLs (one per line). Image apps get the bitmap;
 *     text apps get the URL list.
 *   - Single image: bitmap + its URL in a single ClipboardItem.
 *   - Non-image only: text/plain URLs only; caller shows "已复制链接" toast.
 */
/** Write a text/plain-only ClipboardItem with the given URL text. */
async function writeUrlText(textBlob: Blob, count: number): Promise<CopyResult> {
  const item = new ClipboardItem({ "text/plain": Promise.resolve(textBlob) });
  await navigator.clipboard.write([item]);
  return { type: "url", count };
}

export async function copyAssetsToClipboard(
  assets: Asset[],
  selected: Set<string>,
  focusedId: string | null,
): Promise<CopyResult | null> {
  const targets = resolveTargets(assets, selected, focusedId);
  if (!targets) return null;

  // Secure-context precheck: navigator.clipboard.write / ClipboardItem are only
  // defined over https or localhost. Fail with a clear message rather than a
  // cryptic "Cannot read properties of undefined" from the catch path.
  if (!window.isSecureContext || typeof ClipboardItem === "undefined" || !navigator.clipboard) {
    throw new Error("剪贴板不可用（需 https 或 localhost 安全上下文）");
  }

  // All selected items contribute a URL line.
  const urlText = targets.map((a) => absoluteFileUrl(a.id)).join("\n");
  const textBlob = new Blob([urlText], { type: "text/plain" });

  const primary = pickPrimaryImage(targets, focusedId);

  if (primary) {
    // Dual MIME: image/png (primary) + text/plain (all URLs).
    // Pass Promise<Blob> to ClipboardItem so the browser preserves the
    // user-gesture context while the async fetch + PNG conversion runs.
    try {
      const item = new ClipboardItem({
        "image/png": fetchAsPng(primary.id),
        "text/plain": Promise.resolve(textBlob),
      });
      await navigator.clipboard.write([item]);
      return { type: "image", count: targets.length };
    } catch {
      // The bitmap fetch/convert failed (corrupt image, undecodable RAW, an SVG
      // with no intrinsic size, etc). Because both MIME types share one
      // ClipboardItem, that failure would otherwise drop the URL too — so fall
      // back to URL-only, matching the non-image path. The user always gets the
      // link. (A second write may fail in Safari if the gesture is lost; that
      // rejection propagates to the caller's error toast — never silent.)
      return writeUrlText(textBlob, targets.length);
    }
  }

  // No images — write URL text only.
  return writeUrlText(textBlob, targets.length);
}
