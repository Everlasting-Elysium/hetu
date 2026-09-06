import { useRef, useState } from "react";
import type { Board } from "../types";
import { IconBoard, IconPencil, IconPlus, IconTrash } from "./icons";
import styles from "./BoardList.module.css";

interface Props {
  boards: Board[];
  onOpen: (id: string) => void;
  onCreate: () => void;
  onRename: (id: string, name: string) => void;
  onDelete: (id: string) => void;
}

const fmtDate = (iso: string): string =>
  new Date(iso).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });

// Board management view (ViewMode "boards"): a grid of moodboard cards with
// create, inline-rename, delete, and open-to-canvas. Thumbnails are a glyph
// placeholder until board previews exist server-side.
export function BoardList({ boards, onOpen, onCreate, onRename, onDelete }: Props) {
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const skipBlur = useRef(false);

  const commit = () => {
    if (skipBlur.current) {
      skipBlur.current = false;
      setRenamingId(null);
      return;
    }
    if (renamingId && draft.trim()) onRename(renamingId, draft.trim());
    setRenamingId(null);
  };

  return (
    <div className={styles.wrap}>
      <div className={styles.header}>
        <h2 className={styles.title}>图板</h2>
        <button className="btn btn-primary" onClick={onCreate}>
          <IconPlus width={14} height={14} /> 新建图板
        </button>
      </div>

      {boards.length === 0 ? (
        <div className={styles.empty}>
          <IconBoard width={48} height={48} />
          <p>还没有图板，点击上方按钮创建一个。</p>
        </div>
      ) : (
        <div className={styles.grid}>
          {boards.map((b) => (
            <div key={b.id} className={styles.card} onClick={() => onOpen(b.id)}>
              <div className={styles.thumb}>
                <IconBoard width={40} height={40} />
              </div>
              <div className={styles.meta}>
                {renamingId === b.id ? (
                  <input
                    autoFocus
                    className={`input ${styles.rename}`}
                    value={draft}
                    onClick={(e) => e.stopPropagation()}
                    onChange={(e) => setDraft(e.target.value)}
                    onBlur={commit}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") e.currentTarget.blur();
                      if (e.key === "Escape") {
                        skipBlur.current = true;
                        e.currentTarget.blur();
                      }
                    }}
                  />
                ) : (
                  <div className={styles.name} title={b.name}>
                    {b.name}
                  </div>
                )}
                <div className={styles.date}>更新于 {fmtDate(b.updated_at)}</div>
              </div>
              <div className={styles.actions} onClick={(e) => e.stopPropagation()}>
                <button
                  className={styles.action}
                  title="重命名"
                  onClick={() => {
                    setRenamingId(b.id);
                    setDraft(b.name);
                  }}
                >
                  <IconPencil width={14} height={14} />
                </button>
                <button className={styles.action} title="删除" onClick={() => onDelete(b.id)}>
                  <IconTrash width={14} height={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
