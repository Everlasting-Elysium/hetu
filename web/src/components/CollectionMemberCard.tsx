import type { DragEvent } from "react";
import type { CollectionItem } from "../types";
import { thumbUrl } from "../api/client";
import { IconClose, IconStar, KindIcon } from "./icons";
import styles from "./CollectionView.module.css";

interface Props {
  item: CollectionItem;
  isCover: boolean;
  dragOver: boolean;
  onOpen: () => void;
  onRemove: () => void;
  onSetCover: () => void;
  onDragStart: (e: DragEvent) => void;
  onDragOver: (e: DragEvent) => void;
  onDragEnd: () => void;
  onDrop: (e: DragEvent) => void;
}

// A single collection member: a display-only thumbnail card that doubles as a
// reorder drag source/target (native HTML5 DnD, driven by the parent). Clicking
// opens the shared detail modal; the hover actions set the cover or remove the
// member. Deliberately simpler than AssetCard — no rating/color/selection.
export function CollectionMemberCard({
  item,
  isCover,
  dragOver,
  onOpen,
  onRemove,
  onSetCover,
  onDragStart,
  onDragOver,
  onDragEnd,
  onDrop,
}: Props) {
  const showThumb = item.asset_thumb !== "";
  return (
    <div
      className={`${styles.card} ${dragOver ? styles.dragOver : ""}`}
      data-testid="collection-member"
      data-asset-id={item.asset_id}
      title={item.asset_name}
      draggable
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
      onDrop={onDrop}
      onClick={onOpen}
    >
      <div className={styles.thumb}>
        {showThumb ? (
          <img
            src={thumbUrl(item.asset_id)}
            alt={item.asset_name}
            loading="lazy"
            draggable={false}
          />
        ) : (
          <div className={styles.placeholder}>
            <KindIcon kind={item.asset_kind} />
          </div>
        )}
        {isCover && (
          <span className={styles.coverBadge} title="封面">
            <IconStar width={11} height={11} /> 封面
          </span>
        )}
        <div className={styles.actions} onClick={(e) => e.stopPropagation()}>
          <button
            type="button"
            className={styles.action}
            title="设为封面"
            data-testid="set-cover"
            onClick={onSetCover}
          >
            <IconStar width={13} height={13} />
          </button>
          <button
            type="button"
            className={styles.action}
            title="移出合集"
            data-testid="remove-member"
            onClick={onRemove}
          >
            <IconClose width={13} height={13} />
          </button>
        </div>
      </div>
      <span className={styles.name}>{item.asset_name}</span>
    </div>
  );
}
