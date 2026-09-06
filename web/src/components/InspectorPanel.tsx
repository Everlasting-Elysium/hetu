import { useEffect, useState } from "react";
import type { Asset, AssetKind, Tag } from "../types";
import { COLOR_LABELS } from "../types";
import { thumbUrl } from "../api/client";
import { RatingStars } from "./RatingStars";
import { ColorPopover } from "./ColorPicker";
import { KindIcon } from "./icons";
import styles from "./InspectorPanel.module.css";

interface InspectorProps {
  asset: Asset;
  tags: Tag[];
  onRate: (rating: number) => void;
  onColor: (hex: string) => void;
  onNoteChange: (text: string) => void;
  onNoteDelete: () => void;
}

const KIND_LABELS: Record<AssetKind, string> = {
  image: "图片",
  video: "视频",
  audio: "音频",
  model: "模型",
  document: "文档",
  other: "其他",
};

// Human-readable byte size (binary units), e.g. 2.3 MB. No dependency needed.
function formatSize(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  const val = bytes / 1024 ** i;
  return `${i === 0 ? val : val.toFixed(1)} ${units[i]}`;
}

// Maps a stored hex to its DAM label name, falling back to the raw hex.
function colorName(hex: string): string {
  if (!hex) return "无";
  const match = COLOR_LABELS.find((c) => c.hex.toLowerCase() === hex.toLowerCase());
  return match ? match.name : hex.toUpperCase();
}

// Right-side per-asset inspector: preview, metadata, rating, color label,
// tags, and an auto-saving note. Shown by App when exactly one asset is
// selected. stopPropagation keeps in-panel clicks from clearing the selection.
export function InspectorPanel({
  asset,
  tags,
  onRate,
  onColor,
  onNoteChange,
  onNoteDelete,
}: InspectorProps) {
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);
  const [colorOpen, setColorOpen] = useState(false);
  const [note, setNote] = useState(asset.note);

  // Reset transient thumbnail state and the note draft when the asset changes
  // (also re-syncs the draft after a save refetches the asset).
  useEffect(() => {
    setLoaded(false);
    setFailed(false);
  }, [asset.id]);
  useEffect(() => setNote(asset.note), [asset.id, asset.note]);

  const label = asset.display_name || asset.name;
  const ext = asset.ext.replace(".", "");
  const showThumb = asset.thumb !== "" && !failed;

  const commitNote = () => {
    if (note === asset.note) return;
    if (note.trim() === "") {
      if (asset.note !== "") onNoteDelete();
      return;
    }
    onNoteChange(note);
  };

  return (
    <aside className={styles.panel} onClick={(e) => e.stopPropagation()}>
      <div className={styles.thumb}>
        {showThumb ? (
          <img
            key={asset.id}
            src={thumbUrl(asset.id)}
            alt={label}
            decoding="async"
            className={loaded ? styles.loaded : ""}
            onLoad={() => setLoaded(true)}
            onError={() => setFailed(true)}
          />
        ) : (
          <div className={styles.placeholder}>
            <KindIcon kind={asset.kind} />
            <span className={styles.ext}>{ext || asset.kind}</span>
          </div>
        )}
      </div>

      <div className={styles.section}>
        <div className={styles.name} title={label}>
          {label}
        </div>
        <dl className={styles.info}>
          <div className={styles.infoRow}>
            <dt className={styles.label}>类型</dt>
            <dd className={styles.value}>{KIND_LABELS[asset.kind]}</dd>
          </div>
          {asset.width > 0 && asset.height > 0 && (
            <div className={styles.infoRow}>
              <dt className={styles.label}>尺寸</dt>
              <dd className={styles.value}>
                {asset.width} × {asset.height}
              </dd>
            </div>
          )}
          <div className={styles.infoRow}>
            <dt className={styles.label}>大小</dt>
            <dd className={styles.value}>{formatSize(asset.size)}</dd>
          </div>
          <div className={styles.infoRow}>
            <dt className={styles.label}>格式</dt>
            <dd className={styles.value}>{ext ? ext.toUpperCase() : "—"}</dd>
          </div>
        </dl>
      </div>

      <div className={styles.section}>
        <div className={styles.label}>评分</div>
        <RatingStars value={asset.rating} size={16} onChange={onRate} />
      </div>

      <div className={styles.section}>
        <div className={styles.label}>颜色</div>
        <div className={styles.colorRow}>
          <ColorPopover
            open={colorOpen}
            value={asset.color}
            onPick={(hex) => {
              onColor(hex);
              setColorOpen(false);
            }}
          >
            <button
              type="button"
              title="颜色标签"
              className={`${styles.colorDot} ${asset.color ? "" : styles.colorEmpty}`}
              style={asset.color ? { background: asset.color } : undefined}
              onClick={() => setColorOpen((o) => !o)}
            />
          </ColorPopover>
          <span className={styles.value}>{colorName(asset.color)}</span>
        </div>
      </div>

      <div className={styles.section}>
        <div className={styles.label}>标签</div>
        {tags.length > 0 ? (
          <div className={styles.tags}>
            {tags.map((t) => (
              <span key={t.id} className={styles.chip}>
                {t.color && <i className={styles.chipDot} style={{ background: t.color }} />}
                {t.name}
              </span>
            ))}
          </div>
        ) : (
          <span className={styles.empty}>无标签</span>
        )}
      </div>

      <div className={styles.section}>
        <div className={styles.label}>备注</div>
        <textarea
          className={styles.note}
          placeholder="添加备注..."
          value={note}
          onChange={(e) => setNote(e.target.value)}
          onBlur={commitNote}
        />
      </div>
    </aside>
  );
}
