import type { ComponentType } from "react";
import type { SVGProps } from "react";
import type { ViewMode } from "../types";
import { IconGallery, IconGrid, IconImmersive, IconMasonry } from "./icons";
import styles from "./SearchBar.module.css";

interface Props {
  view: ViewMode;
  onChange: (v: ViewMode) => void;
}

interface Option {
  value: ViewMode;
  label: string;
  Icon: ComponentType<SVGProps<SVGSVGElement>>;
  testid: string;
}

const OPTIONS: Option[] = [
  { value: "grid", label: "网格", Icon: IconGrid, testid: "view-grid" },
  { value: "waterfall", label: "瀑布流", Icon: IconMasonry, testid: "view-waterfall" },
  { value: "gallery", label: "画廊", Icon: IconGallery, testid: "view-gallery" },
  { value: "immersive", label: "沉浸", Icon: IconImmersive, testid: "view-immersive" },
];

// Segmented control for the four browse layouts. Reuses the SearchBar mode-toggle
// styling so it reads as a sibling of the keyword/color search switch.
export function ViewSwitcher({ view, onChange }: Props) {
  return (
    <div className={styles.modes} role="group" aria-label="视图模式">
      {OPTIONS.map(({ value, label, Icon, testid }) => (
        <button
          key={value}
          type="button"
          data-testid={testid}
          className={`${styles.mode} ${view === value ? styles.modeOn : ""}`}
          onClick={() => onChange(value)}
          title={label}
        >
          <Icon width={14} height={14} /> {label}
        </button>
      ))}
    </div>
  );
}
