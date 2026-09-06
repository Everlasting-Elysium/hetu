import { useRef } from "react";
import { Group, Image as KonvaImage, Rect } from "react-konva";
import type Konva from "konva";
import type { BoardItem } from "../types";
import { boardTheme } from "./boardTheme";

interface Props {
  item: BoardItem;
  image: HTMLImageElement | undefined;
  selected: boolean;
  onSelect: () => void;
  onChange: (patch: Partial<BoardItem>) => void;
}

// Smallest a resize may shrink an item to, in world units.
const MIN_SIZE = 24;

// One placed asset on the canvas: a Konva Group (the transform target, keyed by
// its board-item id) wrapping a backing Rect and the thumbnail. Drag and
// transform are Konva-native; the group's scale from a resize is baked back
// into w/h so persisted geometry stays scale-free.
export function BoardCanvasItem({ item, image, selected, onSelect, onChange }: Props) {
  const ref = useRef<Konva.Group>(null);
  const theme = boardTheme();

  const handleDragEnd = () => {
    const node = ref.current;
    if (node) onChange({ x: node.x(), y: node.y() });
  };

  const handleTransformEnd = () => {
    const node = ref.current;
    if (!node) return;
    const scaleX = node.scaleX();
    const scaleY = node.scaleY();
    node.scaleX(1);
    node.scaleY(1);
    onChange({
      x: node.x(),
      y: node.y(),
      w: Math.max(MIN_SIZE, item.w * scaleX),
      h: Math.max(MIN_SIZE, item.h * scaleY),
      rotation: node.rotation(),
    });
  };

  return (
    <Group
      id={item.id}
      ref={ref}
      x={item.x}
      y={item.y}
      rotation={item.rotation}
      draggable
      onMouseDown={onSelect}
      onTap={onSelect}
      onDragEnd={handleDragEnd}
      onTransformEnd={handleTransformEnd}
    >
      <Rect
        width={item.w}
        height={item.h}
        cornerRadius={4}
        fill={theme.itemBg}
        stroke={selected ? theme.accent : theme.border}
        strokeWidth={selected ? 2 : 1}
      />
      {image && <KonvaImage image={image} width={item.w} height={item.h} cornerRadius={4} />}
    </Group>
  );
}
