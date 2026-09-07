import type Konva from "konva";

// The tight world-space box that encloses every placed item (notes + assets),
// used both to preview export dimensions and to crop the exported PNG.
export interface ContentRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

// Every layer child except the selection Transformer overlay — the overlay is
// chrome, never part of the exported board.
function contentNodes(layer: Konva.Layer): Konva.Node[] {
  return layer.getChildren((n) => n.getClassName() !== "Transformer");
}

// Computes the content bounding box in the layer's own (unscaled) coordinate
// space, so it is independent of the current zoom/pan. Returns null when the
// board is empty.
export function boardContentRect(stage: Konva.Stage | null): ContentRect | null {
  const layer = stage?.getLayers()[0];
  if (!layer) return null;
  const nodes = contentNodes(layer);
  if (nodes.length === 0) return null;

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const node of nodes) {
    const r = node.getClientRect({ relativeTo: layer });
    minX = Math.min(minX, r.x);
    minY = Math.min(minY, r.y);
    maxX = Math.max(maxX, r.x + r.width);
    maxY = Math.max(maxY, r.y + r.height);
  }
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
}

// Exports the board content to a downloaded PNG. The stage transform is reset to
// identity for the capture so the world-space content box maps 1:1 to pixels —
// `pixelRatio` alone then sets the output resolution (1x / 2x / 4x). The live
// transform and Transformer visibility are restored before returning, and the
// offscreen capture never touches the on-screen canvas.
export function exportBoard(stage: Konva.Stage, pixelRatio: number): void {
  const rect = boardContentRect(stage);
  const layer = stage.getLayers()[0];
  if (!rect || !layer) return;

  const tr = layer.findOne("Transformer");
  tr?.visible(false);

  const prev = { scale: stage.scaleX(), x: stage.x(), y: stage.y() };
  stage.scale({ x: 1, y: 1 });
  stage.position({ x: 0, y: 0 });
  const dataURL = stage.toDataURL({
    x: rect.x,
    y: rect.y,
    width: rect.w,
    height: rect.h,
    pixelRatio,
  });
  stage.scale({ x: prev.scale, y: prev.scale });
  stage.position({ x: prev.x, y: prev.y });

  tr?.visible(true);

  const link = document.createElement("a");
  link.download = "board-export.png";
  link.href = dataURL;
  link.click();
}
