import { useEffect, useState } from "react";
import type { Asset, Swatch } from "../types";
import { api } from "../api/client";
import { IconClose } from "./icons";
import styles from "./PaletteEditor.module.css";

interface Props {
  asset: Asset;
  // Clicking a swatch in view mode triggers a color search (issue #88).
  onColorSearch?: ((hex: string) => void) | undefined;
}

// The value the add-swatch picker opens on before the user chooses one.
const DEFAULT_PICK = "#888888";

// The asset's extracted palette (issue #16) with hand-editing (issue #62). View
// mode keeps the read-only strip whose swatches trigger a color search (#88); an
// edit toggle swaps in per-swatch color pickers, a delete button, and an add
// button. Persisting on blur (a native color input fires change continuously
// while dragging) keeps each edit to a single request. Audio carries no
// meaningful palette (its thumbnail is a waveform), so it renders nothing.
export function PaletteEditor({ asset, onColorSearch }: Props) {
  const [palette, setPalette] = useState<Swatch[]>([]);
  const [editing, setEditing] = useState(false);
  const canPalette = asset.kind !== "audio";

  useEffect(() => {
    setPalette([]);
    setEditing(false);
    if (!canPalette) return;
    let stale = false;
    api
      .assetColors(asset.id)
      .then((s) => {
        if (!stale) setPalette(s);
      })
      .catch(() => {});
    return () => {
      stale = true;
    };
  }, [asset.id, canPalette]);

  if (!canPalette) return null;

  const add = (hex: string) => {
    api.addAssetColor(asset.id, hex).then(setPalette).catch(() => {});
  };
  const update = (ord: number, hex: string) => {
    api.updateAssetColor(asset.id, ord, hex).then(setPalette).catch(() => {});
  };
  const remove = (ord: number) => {
    api.deleteAssetColor(asset.id, ord).then(setPalette).catch(() => {});
  };

  return (
    <div className={styles.wrap} data-testid="palette-editor">
      <div className={styles.head}>
        <span className={styles.heading}>调色板</span>
        <button
          type="button"
          className={styles.toggle}
          data-testid="palette-edit-toggle"
          aria-pressed={editing}
          onClick={() => setEditing((e) => !e)}
        >
          {editing ? "完成" : "编辑"}
        </button>
      </div>

      {editing ? (
        <div className={styles.chips} data-testid="palette-chips">
          {palette.map((s, i) => (
            <div key={`${s.hex}-${i}`} className={styles.chip}>
              <label className={styles.chipColor} style={{ background: s.hex }} title={`调整 ${s.hex}`}>
                <input
                  type="color"
                  className={styles.picker}
                  defaultValue={s.hex}
                  data-testid="palette-color-input"
                  onBlur={(e) => update(i, e.target.value)}
                />
              </label>
              <button
                type="button"
                className={styles.chipDelete}
                title="删除色卡"
                data-testid="palette-delete"
                onClick={(e) => {
                  e.stopPropagation();
                  remove(i);
                }}
              >
                <IconClose width={9} height={9} />
              </button>
            </div>
          ))}
          <label className={styles.addChip} title="新增色卡" data-testid="palette-add">
            +
            <input
              type="color"
              className={styles.picker}
              defaultValue={DEFAULT_PICK}
              data-testid="palette-add-input"
              onBlur={(e) => add(e.target.value)}
            />
          </label>
        </div>
      ) : palette.length > 0 ? (
        <div className={styles.palette} data-testid="palette-strip">
          {palette.map((s, i) => (
            <button
              key={`${s.hex}-${i}`}
              type="button"
              title={s.hex}
              className={styles.paletteSwatch}
              style={{ background: s.hex, flex: s.weight }}
              onClick={() => onColorSearch?.(s.hex)}
            />
          ))}
        </div>
      ) : (
        <div className={styles.empty}>暂无调色板，点击“编辑”添加</div>
      )}
    </div>
  );
}
