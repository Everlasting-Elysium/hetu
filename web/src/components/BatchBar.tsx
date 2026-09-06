import { useState } from "react";
import { isLibraryView } from "../types";
import type { Folder, Tag, ViewMode } from "../types";
import { RatingStars } from "./RatingStars";
import { ColorSwatches } from "./ColorPicker";
import { IconClose, IconDroplet, IconFolder, IconRestore, IconStar, IconTag, IconTrash } from "./icons";
import styles from "./BatchBar.module.css";

interface Props {
  count: number;
  view: ViewMode;
  folders: Folder[];
  tags: Tag[];
  onClear: () => void;
  onTag: (tagId: string) => void;
  onRate: (rating: number) => void;
  onColor: (hex: string) => void;
  onMove: (folderId: string) => void;
  onTrash: () => void;
  onRestore: () => void;
}

type Menu = "tag" | "rate" | "color" | "move" | null;

// Floating action bar shown while assets are selected. Adapts to the current
// view: library selections expose tag/rate/color/move/trash; trash selections
// expose restore.
export function BatchBar(p: Props) {
  const [menu, setMenu] = useState<Menu>(null);
  if (p.count === 0) return null;

  const toggle = (m: Menu) => setMenu((cur) => (cur === m ? null : m));
  const close = () => setMenu(null);

  return (
    <div className={styles.bar} onClick={(e) => e.stopPropagation()}>
      <span className={styles.count}>
        已选 <b>{p.count}</b> 项
      </span>
      <div className={styles.sep} />

      {isLibraryView(p.view) ? (
        <>
          <div className={styles.menuWrap}>
            <button className="btn btn-ghost" onClick={() => toggle("tag")}>
              <IconTag width={14} height={14} /> 标签
            </button>
            {menu === "tag" && (
              <div className={styles.menu}>
                <div className={styles.menuTitle}>添加标签</div>
                {p.tags.length === 0 && <div className={styles.menuEmpty}>暂无标签</div>}
                {p.tags.map((t) => (
                  <button
                    key={t.id}
                    className={styles.menuItem}
                    onClick={() => {
                      p.onTag(t.id);
                      close();
                    }}
                  >
                    <i style={{ background: t.color || "var(--text-muted)" }} />
                    {t.name}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className={styles.menuWrap}>
            <button className="btn btn-ghost" onClick={() => toggle("rate")}>
              <IconStar width={14} height={14} /> 评分
            </button>
            {menu === "rate" && (
              <div className={styles.menu}>
                <div className={styles.menuTitle}>设置评分</div>
                <div className={styles.rateRow}>
                  <RatingStars
                    value={0}
                    size={18}
                    onChange={(r) => {
                      p.onRate(r);
                      close();
                    }}
                  />
                </div>
              </div>
            )}
          </div>

          <div className={styles.menuWrap}>
            <button className="btn btn-ghost" onClick={() => toggle("color")}>
              <IconDroplet width={14} height={14} /> 颜色
            </button>
            {menu === "color" && (
              <div className={styles.menu}>
                <div className={styles.menuTitle}>颜色标签</div>
                <ColorSwatches
                  value=""
                  showClear
                  onPick={(hex) => {
                    p.onColor(hex);
                    close();
                  }}
                />
              </div>
            )}
          </div>

          <div className={styles.menuWrap}>
            <button className="btn btn-ghost" onClick={() => toggle("move")}>
              <IconFolder width={14} height={14} /> 移动
            </button>
            {menu === "move" && (
              <div className={styles.menu}>
                <div className={styles.menuTitle}>移动到</div>
                <button
                  className={styles.menuItem}
                  onClick={() => {
                    p.onMove("");
                    close();
                  }}
                >
                  <IconFolder width={14} height={14} /> 根目录
                </button>
                {p.folders.map((f) => (
                  <button
                    key={f.id}
                    className={styles.menuItem}
                    onClick={() => {
                      p.onMove(f.id);
                      close();
                    }}
                  >
                    <IconFolder width={14} height={14} /> {f.name}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className={styles.sep} />
          <button className="btn btn-danger" onClick={p.onTrash}>
            <IconTrash width={14} height={14} /> 移入回收站
          </button>
        </>
      ) : (
        <button className="btn btn-primary" onClick={p.onRestore}>
          <IconRestore width={14} height={14} /> 恢复
        </button>
      )}

      <div className={styles.sep} />
      <button className="btn btn-ghost btn-icon" title="取消选择" onClick={p.onClear}>
        <IconClose width={15} height={15} />
      </button>
    </div>
  );
}
