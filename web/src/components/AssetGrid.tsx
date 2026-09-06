import type { Asset } from "../types";
import type { Selection } from "../hooks/useSelection";
import { AssetCard } from "./AssetCard";
import { KindIcon } from "./icons";
import styles from "./AssetGrid.module.css";

interface Props {
  assets: Asset[];
  loading: boolean;
  error: string | null;
  selection: Selection;
  emptyHint: string;
  onRate: (id: string, rating: number) => void;
  onColor: (id: string, hex: string) => void;
  onDetail: (id: string) => void;
}

export function AssetGrid({
  assets,
  loading,
  error,
  selection,
  emptyHint,
  onRate,
  onColor,
  onDetail,
}: Props) {
  if (loading && assets.length === 0)
    return (
      <div className={styles.state}>
        <div className={styles.spinner} />
      </div>
    );

  if (error)
    return (
      <div className={styles.state}>
        <div className={`${styles.empty} ${styles.error}`}>
          <h3>加载失败</h3>
          <p>{error}</p>
        </div>
      </div>
    );

  if (assets.length === 0)
    return (
      <div className={styles.state}>
        <div className={styles.empty}>
          <KindIcon kind="image" width={48} height={48} />
          <h3>暂无素材</h3>
          <p>{emptyHint}</p>
        </div>
      </div>
    );

  return (
    <div className={styles.grid}>
      {assets.map((a) => (
        <AssetCard
          key={a.id}
          asset={a}
          selected={selection.isSelected(a.id)}
          onSelect={(e) => selection.select(a.id, e)}
          onToggleCheck={() => selection.toggle(a.id)}
          onRate={(r) => onRate(a.id, r)}
          onColor={(hex) => onColor(a.id, hex)}
          onDetail={() => onDetail(a.id)}
        />
      ))}
    </div>
  );
}
