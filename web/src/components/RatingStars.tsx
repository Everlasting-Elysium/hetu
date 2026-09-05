import { useState } from "react";
import { IconStar } from "./icons";
import styles from "./RatingStars.module.css";

interface Props {
  value: number;
  size?: number;
  readOnly?: boolean;
  onChange?: (rating: number) => void;
}

// 0-5 star widget. Clicking the current rating again clears it to 0.
export function RatingStars({ value, size = 13, readOnly, onChange }: Props) {
  const [hover, setHover] = useState(0);
  const shown = hover || value;

  return (
    <div
      className={`${styles.stars} ${readOnly ? styles.readonly : ""}`}
      onMouseLeave={() => setHover(0)}
    >
      {[1, 2, 3, 4, 5].map((n) => (
        <button
          key={n}
          type="button"
          className={`${styles.star} ${n <= shown ? styles.on : ""}`}
          title={`${n} 星`}
          onMouseEnter={() => !readOnly && setHover(n)}
          onClick={(e) => {
            e.stopPropagation();
            onChange?.(n === value ? 0 : n);
          }}
        >
          <IconStar width={size} height={size} />
        </button>
      ))}
    </div>
  );
}
