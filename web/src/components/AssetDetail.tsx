import { Suspense, lazy, useEffect } from "react";
import type { Asset, AssetKind } from "../types";
import { fileUrl, thumbUrl } from "../api/client";
import { IconClose, KindIcon } from "./icons";
import { VideoPlayer } from "./VideoPlayer";
import styles from "./AssetDetail.module.css";

// The 3D viewer bundles model-viewer + three.js (~1 MB). Load it lazily so it is
// fetched only when a user actually opens a 3D model, keeping the initial app
// bundle lean for the common image/video/audio cases.
const ModelViewer = lazy(() =>
  import("./ModelViewer").then((m) => ({ default: m.ModelViewer })),
);

interface Props {
  asset: Asset | null;
  onClose: () => void;
}

const KIND_LABELS: Record<AssetKind, string> = {
  image: "图片",
  video: "视频",
  audio: "音频",
  model: "模型",
  document: "文档",
  other: "其他",
};

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(1)} ${units[unit]}`;
}

// Kind-specific preview. Video uses the custom VideoPlayer; audio/image use
// native elements. Media streams from the Range-enabled DAM /file endpoint
// (by asset id) so scrubbing/seeking works.
function AssetMedia({ asset }: { asset: Asset }) {
  const label = asset.display_name || asset.name;
  switch (asset.kind) {
    case "image":
      return (
        <a
          className={styles.imageLink}
          href={fileUrl(asset.id)}
          target="_blank"
          rel="noreferrer"
          title="查看原图"
        >
          <img className={styles.imagePreview} src={thumbUrl(asset.id)} alt={label} />
        </a>
      );
    case "video":
      return <VideoPlayer asset={asset} />;
    case "audio":
      return (
        <div className={styles.audio}>
          {asset.thumb ? (
            <img className={styles.waveform} src={thumbUrl(asset.id)} alt={label} />
          ) : (
            <div className={styles.audioCover}>
              <KindIcon kind="audio" width={72} height={72} />
            </div>
          )}
          <audio className={styles.audioPlayer} src={fileUrl(asset.id)} controls />
        </div>
      );
    case "model":
      return (
        <Suspense
          fallback={
            <div className={styles.modelLoading}>
              <div className={styles.spinner} />
            </div>
          }
        >
          <ModelViewer key={asset.id} asset={asset} />
        </Suspense>
      );
    default:
      return (
        <div className={styles.fallback}>
          <KindIcon kind={asset.kind} width={72} height={72} />
          <a
            className="btn btn-primary"
            href={fileUrl(asset.id)}
            target="_blank"
            rel="noreferrer"
            download
          >
            下载文件
          </a>
        </div>
      );
  }
}

export function AssetDetail({ asset, onClose }: Props) {
  // Escape-to-close. Effect runs unconditionally (rules-of-hooks); the guard
  // keeps the listener off while no asset is open.
  useEffect(() => {
    if (!asset) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [asset, onClose]);

  if (!asset) return null;

  const label = asset.display_name || asset.name;
  const dims =
    asset.width > 0 && asset.height > 0 ? `${asset.width} × ${asset.height}` : null;

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.panel} onClick={(e) => e.stopPropagation()}>
        <button type="button" className={styles.close} title="关闭" onClick={onClose}>
          <IconClose />
        </button>

        <h2 className={styles.title} title={label}>
          {label}
        </h2>

        <div className={styles.media}>
          <AssetMedia asset={asset} />
        </div>

        <dl className={styles.info}>
          <dt>类型</dt>
          <dd>{KIND_LABELS[asset.kind]}</dd>
          <dt>大小</dt>
          <dd>{formatSize(asset.size)}</dd>
          {dims && (
            <>
              <dt>尺寸</dt>
              <dd>{dims}</dd>
            </>
          )}
          <dt>索引时间</dt>
          <dd>{new Date(asset.indexed_at).toLocaleString()}</dd>
          <dt>路径</dt>
          <dd className={styles.path}>{asset.path}</dd>
        </dl>
      </div>
    </div>
  );
}
