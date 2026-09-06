// Shared types mirroring the DAM backend JSON contract
// (internal/plugins/dam). Field names match the Go `json:"..."` tags exactly.

export type AssetKind =
  | "image"
  | "video"
  | "audio"
  | "model"
  | "document"
  | "other";

export interface Asset {
  id: string;
  kind: AssetKind;
  name: string;
  ext: string;
  size: number;
  path: string;
  thumb: string;
  width: number;
  height: number;
  indexed_at: string;
  rating: number;
  color: string;
  display_name: string;
  folder_id: string;
  deleted_at?: string;
  missing_at?: string;
}

// Color-search results extend Asset with the matched swatch + distance.
export interface ColorMatch extends Asset {
  match_hex: string;
  color_distance: number;
}

export interface Folder {
  id: string;
  name: string;
  parent_id: string;
  path: string;
}

export interface Tag {
  id: string;
  name: string;
  color: string;
  parent_id: string;
}

export interface NewFolder {
  name: string;
  parent_id?: string;
  path?: string;
}

export interface NewTag {
  name: string;
  color?: string;
  parent_id?: string;
}

// Which dataset the main grid is showing.
export type ViewMode = "library" | "trash" | "missing";

// Active filter/search state driving the asset query.
export interface Query {
  folderId: string | null;
  tagId: string | null;
  keyword: string;
  colorHex: string | null;
}

export const EMPTY_QUERY: Query = {
  folderId: null,
  tagId: null,
  keyword: "",
  colorHex: null,
};

// Standard DAM color labels — value maps to a CSS var in variables.css.
export interface ColorLabel {
  name: string;
  hex: string;
}

export const COLOR_LABELS: ColorLabel[] = [
  { name: "红", hex: "#e5484d" },
  { name: "橙", hex: "#f76b15" },
  { name: "黄", hex: "#f5c518" },
  { name: "绿", hex: "#46a758" },
  { name: "蓝", hex: "#4f8ff7" },
  { name: "紫", hex: "#8e6fe8" },
];
