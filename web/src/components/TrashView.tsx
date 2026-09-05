import { IconTrash } from "./icons";
import styles from "./TrashView.module.css";

interface Props {
  count: number;
  onEmpty: () => void;
}

// Header bar for the trash view. Per-item restore lives in the BatchBar via
// selection; this exposes the destructive "empty all" purge.
export function TrashView({ count, onEmpty }: Props) {
  return (
    <div className={styles.bar}>
      <div className={styles.info}>
        <IconTrash width={15} height={15} />
        回收站 · <b>{count}</b> 项
      </div>
      <span className="muted" style={{ fontSize: "var(--fs-sm)" }}>
        选择项目后可恢复；清空将永久删除
      </span>
      <div className={styles.spacer} />
      <button className="btn btn-danger" onClick={onEmpty} disabled={count === 0}>
        <IconTrash width={14} height={14} /> 清空回收站
      </button>
    </div>
  );
}
