import { useEffect, useRef, useState } from "react";
import type { Asset, ColorMatch } from "../types";
import { fileUrl, thumbUrl } from "../api/client";
import { RatingStars } from "./RatingStars";
import { ColorPopover } from "./ColorPicker";
import { IconClose, KindIcon } from "./icons";
import styles from "./AssetCard.module.css";

interface Props {
  asset: Asset | ColorMatch;
  selected: boolean;
  // Keyboard cursor: a lighter highlight than `selected`, driven by arrow-key nav.
  focused?: boolean;
  onSelect: (e: React.MouseEvent) => void;
  onToggleCheck: () => void;
  onRate: (rating: number) => void;
  onColor: (hex: string) => void;
  onDetail: () => void;
  // When set (waterfall), the thumb renders at this natural ratio instead of 1:1.
  aspectRatio?: number;
}

const isMatch = (a: Asset | ColorMatch): a is ColorMatch => "match_hex" in a;

export function AssetCard({ asset, selected, focused, onSelect, onToggleCheck, onRate, onColor, onDetail, aspectRatio }: Props) {
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);
  const [colorOpen, setColorOpen] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [previewFailed, setPreviewFailed] = useState(false);
  const hoverTimer = useRef<number | null>(null);
  const label = asset.display_name || asset.name;
  const showThumb = asset.thumb !== "" && !failed;
  const showPreview = hovered && asset.kind === "video" && !previewFailed;

  // Delay the hover-preview mount. A quick double-click (open detail) must not be
  // interrupted by the re-render that mounts the autoplaying <video>: if that
  // re-render lands between the two clicks the browser drops the dblclick and the
  // detail modal never opens. A short intent delay keeps double-click reliable.
  const startHover = () => {
    hoverTimer.current = window.setTimeout(() => setHovered(true), 380);
  };
  const endHover = () => {
    if (hoverTimer.current !== null) {
      clearTimeout(hoverTimer.current);
      hoverTimer.current = null;
    }
    setHovered(false);
  };
  useEffect(
    () => () => {
      if (hoverTimer.current !== null) clearTimeout(hoverTimer.current);
    },
    [],
  );

  return (
    <div
      className={`${styles.card} ${selected ? styles.selected : ""} ${focused ? styles.focused : ""}`}
      data-testid="asset-card"
      onClick={onSelect}
      onDoubleClick={onDetail}
      onMouseEnter={startHover}
      onMouseLeave={endHover}
    >
      <div
        className={styles.thumb}
        style={aspectRatio ? { aspectRatio: String(aspectRatio) } : undefined}
      >
        {showThumb ? (
          <img
            src={thumbUrl(asset.id)}
            alt={label}
            loading="lazy"
            decoding="async"
            className={loaded ? styles.loaded : ""}
            onLoad={() => setLoaded(true)}
            onError={() => setFailed(true)}
          />
        ) : (
          <div className={styles.placeholder}>
            <KindIcon kind={asset.kind} />
            <span className={styles.ext}>{asset.ext.replace(".", "") || asset.kind}</span>
          </div>
        )}

        {showPreview && (
          <video
            className={styles.preview}
            src={fileUrl(asset.id)}
            poster={thumbUrl(asset.id)}
            muted
            loop
            autoPlay
            playsInline
            preload="none"
            onError={() => setPreviewFailed(true)}
          />
        )}

        <button
          type="button"
          className={`${styles.check} ${selected ? styles.on : ""}`}
          title="选择"
          onClick={(e) => {
            e.stopPropagation();
            onToggleCheck();
          }}
        >
          {selected && <IconClose width={12} height={12} style={{ transform: "rotate(45deg)" }} />}
        </button>

        {isMatch(asset) && (
          <div className={styles.distance} title={`ΔE ${asset.color_distance}`}>
            <i style={{ background: asset.match_hex }} />
            {asset.color_distance}
          </div>
        )}
      </div>

      <div className={styles.meta}>
        <div className={styles.name} title={label}>
          {label}
        </div>
        <div className={styles.row}>
          <RatingStars value={asset.rating} onChange={onRate} />
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
              className={`${styles.dot} ${asset.color ? "" : styles.dotEmpty}`}
              style={asset.color ? { background: asset.color } : undefined}
              onClick={(e) => {
                e.stopPropagation();
                setColorOpen((o) => !o);
              }}
            />
          </ColorPopover>
        </div>
      </div>
    </div>
  );
}
