import { useEffect, useRef, useState } from "react";
import type { BoardItem } from "../types";
import styles from "./BoardCanvas.module.css";

interface Props {
  item: BoardItem;
  scale: number;
  pos: { x: number; y: number };
  onCommit: (text: string) => void;
  onCancel: () => void;
}

// An HTML textarea floated exactly over a note's Konva rect (Konva cannot host
// DOM children), so double-click editing feels in place. The screen box is the
// stage pan/zoom applied to the note's world geometry. Commits on blur or
// Cmd/Ctrl+Enter and discards on Escape; a one-shot guard stops the unmount
// blur from firing a second, conflicting resolution.
export function BoardNoteEditor({ item, scale, pos, onCommit, onCancel }: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const done = useRef(false);
  const [text, setText] = useState(item.text ?? "");

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.focus();
    el.select();
  }, []);

  const commit = () => {
    if (done.current) return;
    done.current = true;
    onCommit(text);
  };
  const cancel = () => {
    if (done.current) return;
    done.current = true;
    onCancel();
  };

  return (
    <textarea
      ref={ref}
      className={styles.noteEditor}
      style={{
        left: pos.x + item.x * scale,
        top: pos.y + item.y * scale,
        width: item.w * scale,
        height: item.h * scale,
        transform: item.rotation ? `rotate(${item.rotation}deg)` : undefined,
        transformOrigin: "0 0",
      }}
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          cancel();
        } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault();
          commit();
        }
      }}
    />
  );
}
