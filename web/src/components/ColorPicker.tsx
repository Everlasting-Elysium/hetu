import { COLOR_LABELS } from "../types";
import { IconClose } from "./icons";
import styles from "./ColorPicker.module.css";

interface SwatchesProps {
  value: string;
  onPick: (hex: string) => void;
  showClear?: boolean;
}

// Inline row of standard DAM color labels. `value` highlights the active one.
export function ColorSwatches({ value, onPick, showClear }: SwatchesProps) {
  const norm = value.toLowerCase();
  return (
    <div className={styles.swatches}>
      {COLOR_LABELS.map((c) => (
        <button
          key={c.hex}
          type="button"
          title={c.name}
          className={`${styles.swatch} ${norm === c.hex.toLowerCase() ? styles.active : ""}`}
          style={{ background: c.hex }}
          onClick={(e) => {
            e.stopPropagation();
            onPick(c.hex);
          }}
        />
      ))}
      {showClear && (
        <button
          type="button"
          title="清除颜色"
          className={styles.clear}
          onClick={(e) => {
            e.stopPropagation();
            onPick("");
          }}
        >
          <IconClose width={11} height={11} />
        </button>
      )}
    </div>
  );
}

interface PopoverProps extends SwatchesProps {
  open: boolean;
  children: React.ReactNode;
}

// A trigger + floating swatch popover.
export function ColorPopover({ open, children, ...rest }: PopoverProps) {
  return (
    <div className={styles.wrap}>
      {children}
      {open && (
        <div className={styles.pop} onClick={(e) => e.stopPropagation()}>
          <ColorSwatches {...rest} showClear />
        </div>
      )}
    </div>
  );
}
