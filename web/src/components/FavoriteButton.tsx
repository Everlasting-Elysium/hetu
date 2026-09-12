import { IconHeart } from "./icons";
import styles from "./FavoriteButton.module.css";

interface Props {
  value: boolean;
  size?: number;
  readOnly?: boolean;
  onChange?: (favorite: boolean) => void;
}

// Heart toggle mirroring RatingStars' click-to-set pattern (issue #62): clicking
// flips the current favorite state. Filled when favorited, outlined otherwise.
export function FavoriteButton({ value, size = 14, readOnly, onChange }: Props) {
  return (
    <button
      type="button"
      className={`${styles.fav} ${value ? styles.on : ""} ${readOnly ? styles.readonly : ""}`}
      title={value ? "取消收藏" : "收藏"}
      aria-pressed={value}
      data-testid="favorite-toggle"
      onClick={(e) => {
        e.stopPropagation();
        if (!readOnly) onChange?.(!value);
      }}
    >
      <IconHeart width={size} height={size} />
    </button>
  );
}
