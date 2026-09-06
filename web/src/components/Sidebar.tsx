import { useState } from "react";
import type { Folder, Tag } from "../types";
import { IconAlert, IconFolder, IconGrid, IconPlus, IconTag, IconTrash } from "./icons";
import styles from "./Sidebar.module.css";

interface Props {
  folders: Folder[];
  tags: Tag[];
  activeFolder: string | null;
  activeTag: string | null;
  onPickFolder: (id: string | null) => void;
  onPickTag: (id: string | null) => void;
  onCreateFolder: (name: string) => void;
  onDeleteFolder: (id: string) => void;
  onCreateTag: (name: string) => void;
  onDeleteTag: (id: string) => void;
  missingCount: number;
  onPickMissing: () => void;
  activeMissing: boolean;
}

// Inline "add" form toggled per section.
function AddForm({ placeholder, onSubmit }: { placeholder: string; onSubmit: (v: string) => void }) {
  const [v, setV] = useState("");
  return (
    <form
      className={styles.form}
      onSubmit={(e) => {
        e.preventDefault();
        if (v.trim()) {
          onSubmit(v.trim());
          setV("");
        }
      }}
    >
      <input
        autoFocus
        className="input"
        placeholder={placeholder}
        value={v}
        onChange={(e) => setV(e.target.value)}
      />
      <button type="submit" className="btn btn-primary btn-icon" title="创建">
        <IconPlus width={14} height={14} />
      </button>
    </form>
  );
}

export function Sidebar(p: Props) {
  const [addFolder, setAddFolder] = useState(false);
  const [addTag, setAddTag] = useState(false);
  const allActive = !p.activeFolder && !p.activeTag && !p.activeMissing;

  return (
    <aside className={styles.side}>
      <div className={styles.section}>
        <button
          className={`${styles.item} ${allActive ? styles.active : ""}`}
          onClick={() => {
            p.onPickFolder(null);
            p.onPickTag(null);
          }}
        >
          <IconGrid width={15} height={15} />
          <span className={styles.txt}>全部素材</span>
        </button>
        {p.missingCount > 0 && (
          <button
            className={`${styles.item} ${p.activeMissing ? styles.active : ""}`}
            onClick={p.onPickMissing}
          >
            <IconAlert width={15} height={15} />
            <span className={styles.txt}>丢失文件</span>
            <span className={styles.badge}>{p.missingCount}</span>
          </button>
        )}
      </div>

      <div className={styles.section}>
        <div className={styles.head}>
          <span>文件夹</span>
          <button className={styles.add} title="新建文件夹" onClick={() => setAddFolder((x) => !x)}>
            <IconPlus width={13} height={13} />
          </button>
        </div>
        {addFolder && (
          <AddForm
            placeholder="文件夹名称"
            onSubmit={(v) => {
              p.onCreateFolder(v);
              setAddFolder(false);
            }}
          />
        )}
        {p.folders.map((f) => (
          <button
            key={f.id}
            className={`${styles.item} ${p.activeFolder === f.id ? styles.active : ""}`}
            title={f.path || f.name}
            onClick={() => p.onPickFolder(f.id)}
          >
            <IconFolder width={15} height={15} />
            <span className={styles.txt}>{f.name}</span>
            <span
              className={styles.del}
              title="删除"
              onClick={(e) => {
                e.stopPropagation();
                p.onDeleteFolder(f.id);
              }}
            >
              <IconTrash width={13} height={13} />
            </span>
          </button>
        ))}
      </div>

      <div className={styles.section}>
        <div className={styles.head}>
          <span>标签</span>
          <button className={styles.add} title="新建标签" onClick={() => setAddTag((x) => !x)}>
            <IconPlus width={13} height={13} />
          </button>
        </div>
        {addTag && (
          <AddForm
            placeholder="标签名称"
            onSubmit={(v) => {
              p.onCreateTag(v);
              setAddTag(false);
            }}
          />
        )}
        {p.tags.map((t) => (
          <button
            key={t.id}
            className={`${styles.item} ${p.activeTag === t.id ? styles.active : ""}`}
            onClick={() => p.onPickTag(t.id)}
          >
            {t.color ? (
              <i className={styles.swatch} style={{ background: t.color }} />
            ) : (
              <IconTag width={14} height={14} />
            )}
            <span className={styles.txt}>{t.name}</span>
            <span
              className={styles.del}
              title="删除"
              onClick={(e) => {
                e.stopPropagation();
                p.onDeleteTag(t.id);
              }}
            >
              <IconTrash width={13} height={13} />
            </span>
          </button>
        ))}
      </div>
    </aside>
  );
}
