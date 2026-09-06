import { useEffect, useState } from "react";
import type { ViewMode } from "../types";
import { IconAlert, IconClose, IconDroplet, IconSearch, IconTrash } from "./icons";
import styles from "./SearchBar.module.css";

type SearchKind = "keyword" | "color";

interface Props {
  view: ViewMode;
  trashCount: number;
  missingCount: number;
  onKeyword: (q: string) => void;
  onColor: (hex: string | null) => void;
  onViewChange: (v: ViewMode) => void;
}

// Topbar: keyword/color search modes (300ms debounced), plus library/trash
// view toggle. Switching mode clears the other mode's active query.
export function SearchBar({ view, trashCount, missingCount, onKeyword, onColor, onViewChange }: Props) {
  const [kind, setKind] = useState<SearchKind>("keyword");
  const [text, setText] = useState("");
  const [hex, setHex] = useState("#4f8ff7");
  const [colorActive, setColorActive] = useState(false);

  useEffect(() => {
    if (kind !== "keyword") return;
    const t = setTimeout(() => onKeyword(text), 300);
    return () => clearTimeout(t);
  }, [text, kind, onKeyword]);

  const switchKind = (k: SearchKind) => {
    setKind(k);
    if (k === "keyword") {
      onColor(null);
      setColorActive(false);
    } else {
      onKeyword("");
      setText("");
    }
  };

  const applyColor = (h: string) => {
    setHex(h);
    setColorActive(true);
    onColor(h);
  };

  return (
    <div className={styles.top}>
      <div className={styles.modes}>
        <button
          className={`${styles.mode} ${kind === "keyword" ? styles.modeOn : ""}`}
          onClick={() => switchKind("keyword")}
        >
          <IconSearch width={14} height={14} /> 关键词
        </button>
        <button
          className={`${styles.mode} ${kind === "color" ? styles.modeOn : ""}`}
          onClick={() => switchKind("color")}
        >
          <IconDroplet width={14} height={14} /> 颜色
        </button>
      </div>

      {kind === "keyword" ? (
        <div className={styles.search}>
          <IconSearch width={16} height={16} />
          <input
            className={`input ${styles.field}`}
            placeholder="搜索名称、标签、描述…（FTS5）"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          {text && (
            <button className={styles.clear} title="清除" onClick={() => setText("")}>
              <IconClose width={14} height={14} />
            </button>
          )}
        </div>
      ) : (
        <div className={styles.colorPick}>
          <input
            type="color"
            className={styles.native}
            value={hex}
            onChange={(e) => applyColor(e.target.value)}
          />
          <span className={styles.hex}>{colorActive ? hex : "选择颜色搜索"}</span>
          {colorActive && (
            <button
              className={styles.clear}
              title="清除"
              onClick={() => {
                setColorActive(false);
                onColor(null);
              }}
            >
              <IconClose width={14} height={14} />
            </button>
          )}
        </div>
      )}

      <div className={styles.spacer} />

      <div className={styles.viewToggle}>
        <button
          className={`btn ${view === "library" ? "btn-primary" : "btn-ghost"}`}
          onClick={() => onViewChange("library")}
        >
          素材库
        </button>
        <button
          className={`btn ${view === "trash" ? "btn-primary" : "btn-ghost"}`}
          onClick={() => onViewChange("trash")}
        >
          <IconTrash width={14} height={14} /> 回收站
          {trashCount > 0 && <span className={styles.badge}>{trashCount}</span>}
        </button>
        {missingCount > 0 && (
          <button
            className={`btn ${view === "missing" ? "btn-primary" : "btn-ghost"}`}
            onClick={() => onViewChange("missing")}
          >
            <IconAlert width={14} height={14} /> 丢失文件
            <span className={styles.badge}>{missingCount}</span>
          </button>
        )}
      </div>
    </div>
  );
}
