import { KindIcon } from "./icons";
import styles from "./AssetGrid.module.css";

// Shared loading / error / empty placeholders for the asset views so grid,
// waterfall and gallery render identical states (single source of truth).

export function GridSpinner() {
  return (
    <div className={styles.state}>
      <div className={styles.spinner} />
    </div>
  );
}

export function GridError({ message }: { message: string }) {
  return (
    <div className={styles.state}>
      <div className={`${styles.empty} ${styles.error}`}>
        <h3>加载失败</h3>
        <p>{message}</p>
      </div>
    </div>
  );
}

export function GridEmpty({ hint }: { hint: string }) {
  return (
    <div className={styles.state}>
      <div className={styles.empty}>
        <KindIcon kind="image" width={48} height={48} />
        <h3>暂无素材</h3>
        <p>{hint}</p>
      </div>
    </div>
  );
}
