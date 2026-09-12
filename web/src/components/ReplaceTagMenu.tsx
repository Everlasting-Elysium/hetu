import { useState } from "react";
import type { Tag } from "../types";
import { IconTag } from "./icons";
import styles from "./BatchBar.module.css";

interface Props {
  tags: Tag[];
  open: boolean;
  onToggle: () => void;
  onClose: () => void;
  onReplaceTag: (fromTagId: string, toTagId: string) => void;
}

// The batch "替换标签" action (issue #62): pick a source tag and a target tag,
// then swap them on the selected assets only. Extracted from BatchBar to keep it
// under the 250-LOC ceiling; owns its own from/to select state (self-contained,
// reset on apply) while BatchBar still owns which menu is open.
export function ReplaceTagMenu({ tags, open, onToggle, onClose, onReplaceTag }: Props) {
  const [replaceFrom, setReplaceFrom] = useState("");
  const [replaceTo, setReplaceTo] = useState("");
  const canReplace = replaceFrom !== "" && replaceTo !== "" && replaceFrom !== replaceTo;

  return (
    <div className={styles.menuWrap}>
      <button className="btn btn-ghost" onClick={onToggle}>
        <IconTag width={14} height={14} /> 替换标签
      </button>
      {open && (
        <div className={styles.menu} data-testid="replace-tag-menu">
          <div className={styles.menuTitle}>替换标签</div>
          {tags.length === 0 ? (
            <div className={styles.menuEmpty}>暂无标签</div>
          ) : (
            <div className={styles.replaceForm}>
              <label className={styles.replaceField}>
                从
                <select
                  className="input"
                  data-testid="replace-tag-from"
                  value={replaceFrom}
                  onChange={(e) => setReplaceFrom(e.target.value)}
                >
                  <option value="">选择标签</option>
                  {tags.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className={styles.replaceField}>
                换成
                <select
                  className="input"
                  data-testid="replace-tag-to"
                  value={replaceTo}
                  onChange={(e) => setReplaceTo(e.target.value)}
                >
                  <option value="">选择标签</option>
                  {tags.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name}
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                className="btn btn-primary"
                data-testid="replace-tag-apply"
                disabled={!canReplace}
                onClick={() => {
                  onReplaceTag(replaceFrom, replaceTo);
                  setReplaceFrom("");
                  setReplaceTo("");
                  onClose();
                }}
              >
                替换
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
