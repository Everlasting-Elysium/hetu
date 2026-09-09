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
  note: string;
  deleted_at?: string;
  missing_at?: string;
}

// Color-search results extend Asset with the matched swatch + distance.
export interface ColorMatch extends Asset {
  match_hex: string;
  color_distance: number;
}

// One row of the format facet: an asset kind and its live count in the current
// folder/tag/rating context. Mirrors the kindCount json in internal/plugins/dam.
export interface KindCount {
  kind: AssetKind;
  count: number;
}

// GET /facets response driving the format facet.
export interface Facets {
  kinds: KindCount[];
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

// A collection: a manually-curated, nestable group of assets (issue #55). An
// asset may belong to many collections. `parent_id` is "" for a root-level
// collection; `cover` is the server-resolved cover asset id (an explicit value,
// else the ord-smallest member, else "").
export interface Collection {
  id: string;
  name: string;
  parent_id: string;
  cover: string;
}

// One member of a collection (GET /collections/:id/items), ordered by `ord`
// ascending and enriched server-side with the asset's kind/name/thumb — the same
// pattern as BoardItem's asset_* fields — so the collection view renders each
// member without a per-asset request. Opening the detail modal still fetches the
// full Asset via GET /assets/:id.
export interface CollectionItem {
  asset_id: string;
  ord: number;
  asset_kind: AssetKind;
  asset_name: string;
  asset_thumb: string;
}

export interface NewCollection {
  name: string;
  parent_id?: string;
}

// A moodboard / infinite canvas. `items` is only populated by GET /boards/:id.
export interface Board {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
  items?: BoardItem[];
}

// One item placed on a board, with canvas geometry. Field names match the
// boardItemDTO json tags in internal/plugins/dam/boards.go exactly. `kind`
// distinguishes an asset thumbnail from a free-text note (absent == "asset");
// `text` holds note content; `frame_ms`/`view` pin a video frame / 3D camera.
export interface BoardItem {
  id: string;
  kind?: "asset" | "note";
  asset_id: string;
  text?: string;
  frame_ms?: number | null;
  view?: string;
  // asset_* are resolved server-side (GET /boards/:id, POST items) so an asset
  // item on the board knows its own kind/name/thumb without the asset panel's
  // query being loaded — this is what lets a video/model item reopen its
  // frame/angle picker regardless of the panel's current search/filter.
  asset_kind?: AssetKind;
  asset_name?: string;
  asset_thumb?: string;
  x: number;
  y: number;
  w: number;
  h: number;
  rotation: number;
  z: number;
}

// The minimal asset shape the frame/angle pickers need, resolved from a board
// item's asset_* fields so the pickers never depend on the asset panel query.
export interface PickerAsset {
  id: string;
  name: string;
  thumb?: string;
}

// The active view. Browse layouts (grid/waterfall/gallery/immersive) all show the
// library dataset in different arrangements; trash/missing are distinct datasets.
// "boards" lists moodboards; "board" is the infinite-canvas editor for one board.
// "collection" is the member-grid detail for one collection — the collection tree
// itself lives permanently in the sidebar, so there is no separate list view.
export type ViewMode =
  | "grid"
  | "waterfall"
  | "gallery"
  | "immersive"
  | "trash"
  | "missing"
  | "boards"
  | "board"
  | "collection";

// Layouts that browse the library dataset (as opposed to trash/missing).
export const LIBRARY_LAYOUTS = ["grid", "waterfall", "gallery", "immersive"] as const;
// Layouts persisted as the default browse preference — immersive is a transient
// overlay entered on demand, so it is never stored as the startup view.
export const BROWSE_LAYOUTS = ["grid", "waterfall", "gallery"] as const;
export type BrowseLayout = (typeof BROWSE_LAYOUTS)[number];

export const isLibraryView = (v: ViewMode): boolean =>
  (LIBRARY_LAYOUTS as readonly string[]).includes(v);
export const isBrowseLayout = (v: ViewMode): v is BrowseLayout =>
  (BROWSE_LAYOUTS as readonly string[]).includes(v);

// Active filter/search state driving the asset query. folderId/tagId/kind/
// minRating compose (AND) and narrow server-side; keyword and colorHex are the
// two search modes. kind is the format facet (empty = all formats) and minRating
// is the rating facet (0 = any rating).
export interface Query {
  folderId: string | null;
  tagId: string | null;
  kind: AssetKind[];
  minRating: number;
  keyword: string;
  colorHex: string | null;
}

export const EMPTY_QUERY: Query = {
  folderId: null,
  tagId: null,
  kind: [],
  minRating: 0,
  keyword: "",
  colorHex: null,
};

// One swatch from an asset's extracted palette (GET /assets/{id}/colors).
// Not to be confused with Asset.color, which is the manual Finder-style label.
export interface Swatch {
  hex: string;
  weight: number;
}

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
