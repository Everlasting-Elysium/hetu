import { useRef, useState } from "react";
import type { CompareSlot as SlotData } from "../../hooks/useCompare";
import { IconDownload } from "../icons";
import styles from "./Compare.module.css";

// One comparison slot (issue #127): shows the picked library asset (thumb + name)
// or a transient local file preview (object URL), with a 更换 entry that reveals a
// chooser — either upload a local PNG/JPEG (drag or click) or return to the
// library to re-select. The upload path holds the raw File and feeds /compare
// directly; it deliberately does NOT use useImport/importAsset (that path creates
// a permanent library asset, a different intent from "compare, don't keep").
interface Props {
  label: string;
  slot: SlotData | null;
  onFile: (file: File) => void;
  onReturnToLibrary: () => void;
  testid: string;
}

export function CompareSlot({ label, slot, onFile, onReturnToLibrary, testid }: Props) {
  const [choosing, setChoosing] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const pick = (file: File | null | undefined) => {
    if (!file) return;
    onFile(file);
    setChoosing(false);
  };

  const showChooser = slot === null || choosing;
  const preview = slot === null ? "" : slot.kind === "asset" ? slot.thumb : slot.url;
  const name = slot === null ? "" : slot.kind === "asset" ? slot.name : slot.file.name;

  return (
    <div className={styles.slot} data-testid={testid}>
      <div className={styles.slotLabel}>{label}</div>

      {showChooser ? (
        <div
          className={`${styles.dropZone} ${dragOver ? styles.dropOver : ""}`}
          onDragEnter={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragOver={(e) => e.preventDefault()}
          onDragLeave={() => setDragOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragOver(false);
            pick(e.dataTransfer.files[0]);
          }}
          onClick={() => inputRef.current?.click()}
        >
          <input
            ref={inputRef}
            type="file"
            accept="image/png,image/jpeg"
            hidden
            data-testid={`${testid}-file`}
            onChange={(e) => pick(e.target.files?.[0])}
          />
          <IconDownload width={20} height={20} />
          <span className={styles.dropText}>拖入或点击上传本地图片</span>
          <span className={styles.dropHint}>PNG / JPEG · 仅用于对比，不导入图库</span>
          <div className={styles.dropActions}>
            <button
              type="button"
              className="btn btn-ghost"
              onClick={(e) => {
                e.stopPropagation();
                onReturnToLibrary();
              }}
            >
              从图库选择
            </button>
            {slot && (
              <button
                type="button"
                className="btn btn-ghost"
                onClick={(e) => {
                  e.stopPropagation();
                  setChoosing(false);
                }}
              >
                取消
              </button>
            )}
          </div>
        </div>
      ) : (
        <div className={styles.slotFilled}>
          <img className={styles.slotThumb} src={preview} alt={name} />
          <div className={styles.slotName} data-testid={`${testid}-name`} title={name}>
            {name}
          </div>
          <button
            type="button"
            className="btn btn-ghost"
            data-testid={`${testid}-change`}
            onClick={() => setChoosing(true)}
          >
            更换
          </button>
        </div>
      )}
    </div>
  );
}
