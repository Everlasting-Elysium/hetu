// Typed client for the DAM HTTP API. All calls target `/api/dam/*`; the Vite
// dev server proxies to the Go backend on :8080, and in production the same
// origin serves both the SPA and the API.
import type {
  Asset,
  AssetKind,
  Board,
  BoardItem,
  Collection,
  CollectionItem,
  ColorMatch,
  Facets,
  Folder,
  NewCollection,
  NewFolder,
  NewTag,
  Query,
  Swatch,
  Tag,
} from "../types";

const BASE = "/api/dam";

// The facet filter shared by /assets, /search, and /facets. Empty fields are
// omitted so the server applies only the active facets (issue #75).
export interface AssetFilterParams {
  folder?: string | null;
  tag?: string | null;
  kind?: AssetKind[];
  rating?: number;
}

// Result of a multipart POST /import (issue #89): the stored asset, or
// skipped=true when a content duplicate was skipped. Mirrors importResp in
// internal/plugins/dam/import.go.
export interface ImportResult {
  asset?: Asset;
  skipped: boolean;
}

// Partial body for PATCH /collections/:id (issue #55). With
// exactOptionalPropertyTypes a caller includes only the keys it is changing
// (never an explicit `undefined`); `cover` set to "" clears the manual cover.
export interface CollectionPatch {
  name?: string;
  parent_id?: string;
  cover?: string;
}

// queryFilter maps the composable facets of a Query onto the wire params. It is
// the single Query -> filter bridge shared by useAssets and useFacets, so the
// two never drift on which fields narrow a request.
export function queryFilter(q: Query): AssetFilterParams {
  return { folder: q.folderId, tag: q.tagId, kind: q.kind, rating: q.minRating };
}

// Serializes the shared facets into a query string (folder/tag/kind/rating),
// dropping empties. kind is comma-joined to match the ?kind=a,b backend parser.
function filterParams(f: AssetFilterParams): string {
  const p = new URLSearchParams();
  if (f.folder) p.set("folder", f.folder);
  if (f.tag) p.set("tag", f.tag);
  if (f.kind && f.kind.length > 0) p.set("kind", f.kind.join(","));
  if (f.rating && f.rating > 0) p.set("rating", String(f.rating));
  return p.toString();
}

// Joins a base path with the facet params and pagination into one query string.
function withFilter(prefix: string, f: AssetFilterParams, limit: number, offset: number): string {
  const parts = [prefix, filterParams(f), `limit=${limit}`, `offset=${offset}`].filter(Boolean);
  return parts.join("&");
}

interface ApiError {
  error?: string;
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = init?.body ? { "Content-Type": "application/json" } : {};
  const res = await fetch(BASE + path, { ...init, headers });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as ApiError;
      if (body.error) msg = body.error;
    } catch {
      /* non-JSON error body */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

const body = (data: unknown): RequestInit => ({
  method: "POST",
  body: JSON.stringify(data),
});

const patch = (data: unknown): RequestInit => ({
  method: "PATCH",
  body: JSON.stringify(data),
});

export const thumbUrl = (id: string): string => `${BASE}/assets/${id}/thumb`;

// Streams the original bytes (http.ServeContent, Range-enabled) so <audio>/<video>
// can seek. Keyed by asset id — the DAM endpoint resolves the storage path
// server-side, so the disk path is never exposed and no NAS plugin is required.
export const fileUrl = (id: string): string => `${BASE}/assets/${id}/file`;

// Serves a browser-loadable 3D model (#51): glTF/GLB stream as-is, other
// supported formats are converted to GLB server-side and cached. Feeds the
// <model-viewer> `src` in the asset detail modal.
export const modelUrl = (id: string): string => `${BASE}/assets/${id}/model`;

// Decodes one video frame at millisecond offset `ms` as JPEG (#86), for timeline
// scrubbing / hover previews. Deterministic per (id, ms) and long-cached, so it
// is safe to use directly as an <img> src.
export const frameUrl = (id: string, ms: number): string =>
  `${BASE}/assets/${id}/frame?ms=${ms}`;

export const api = {
  // listAssets and searchKeyword push the folder/tag/kind/rating facets to the
  // server (issue #75), so the client never filters an asset list in memory.
  listAssets: (f: AssetFilterParams = {}, limit = 200, offset = 0) =>
    req<Asset[]>(`/assets?${withFilter("", f, limit, offset)}`),

  searchKeyword: (q: string, f: AssetFilterParams = {}, limit = 200, offset = 0) =>
    req<Asset[]>(`/search?${withFilter(`q=${encodeURIComponent(q)}`, f, limit, offset)}`),

  // facets returns per-kind counts for the format facet, narrowed by the same
  // folder/tag/rating context (the active kind selection is ignored server-side).
  facets: (f: AssetFilterParams = {}) => {
    const qs = filterParams(f);
    return req<Facets>(`/facets${qs ? `?${qs}` : ""}`);
  },

  searchColor: (hex: string, tol = 12, limit = 200) =>
    req<ColorMatch[]>(
      `/search?color=${encodeURIComponent(hex.replace("#", ""))}&tol=${tol}&limit=${limit}`,
    ),

  listFolders: () => req<Folder[]>("/folders"),
  createFolder: (f: NewFolder) => req<Folder>("/folders", body(f)),
  deleteFolder: (id: string) =>
    req<{ deleted: boolean }>(`/folders/${id}`, { method: "DELETE" }),

  listTags: () => req<Tag[]>("/tags"),
  createTag: (t: NewTag) => req<Tag>("/tags", body(t)),
  deleteTag: (id: string) =>
    req<{ deleted: boolean }>(`/tags/${id}`, { method: "DELETE" }),
  // Fetches one asset's full DTO by id (issue #55). The collection view holds only
  // asset ids (GET /collections/:id/items) and needs the complete Asset to open
  // the shared detail modal.
  getAsset: (id: string) => req<Asset>(`/assets/${id}`),
  assetTags: (id: string) => req<Tag[]>(`/assets/${id}/tags`),
  assetColors: (id: string) => req<Swatch[]>(`/assets/${id}/colors`),

  updateNote: (id: string, text: string) =>
    req<{ note: string }>(`/assets/${id}/note`, {
      method: "PUT",
      body: JSON.stringify({ text }),
    }),
  deleteNote: (id: string) =>
    req<{ deleted: boolean }>(`/assets/${id}/note`, { method: "DELETE" }),

  rate: (asset_ids: string[], rating: number) =>
    req<{ updated: number }>("/batch/rate", body({ asset_ids, rating })),
  colorLabel: (asset_ids: string[], color: string) =>
    req<{ updated: number }>("/batch/color", body({ asset_ids, color })),
  move: (asset_ids: string[], folder_id: string) =>
    req<{ moved: number }>("/batch/move", body({ asset_ids, folder_id })),
  trash: (asset_ids: string[]) =>
    req<{ trashed: number }>("/batch/trash", body({ asset_ids })),
  restore: (asset_ids: string[]) =>
    req<{ restored: number }>("/batch/restore", body({ asset_ids })),
  tag: (asset_ids: string[], tag_ids: string[]) =>
    req<{ tagged: number }>("/batch/tag", body({ asset_ids, tag_ids })),
  untag: (asset_ids: string[], tag_id: string) =>
    req<{ untagged: number }>("/batch/untag", body({ asset_ids, tag_id })),
  rename: (asset_ids: string[], display_name: string) =>
    req<{ renamed: number }>(
      "/batch/rename",
      body({ asset_ids, pattern: "", display_name }),
    ),

  listTrash: (limit = 200, offset = 0) =>
    req<Asset[]>(`/trash?limit=${limit}&offset=${offset}`),
  purgeTrash: (retention_days = 0) =>
    req<{ purged: boolean }>(`/trash?retention_days=${retention_days}`, {
      method: "DELETE",
    }),

  listBoards: () => req<Board[]>("/boards"),
  getBoard: (id: string) => req<Board>(`/boards/${id}`),
  createBoard: (name: string) => req<Board>("/boards", body({ name })),
  renameBoard: (id: string, name: string) =>
    req<{ ok: boolean }>(`/boards/${id}`, patch({ name })),
  deleteBoard: (id: string) =>
    req<{ deleted: boolean }>(`/boards/${id}`, { method: "DELETE" }),

  addBoardItem: (id: string, item: Omit<BoardItem, "id">) =>
    req<BoardItem>(`/boards/${id}/items`, body(item)),
  // A free-text note carries no asset; kind="note" and the geometry are enough
  // for the server to store it alongside asset items on the same board.
  addBoardNote: (
    boardId: string,
    note: { text: string; x: number; y: number; w: number; h: number },
  ) =>
    req<BoardItem>(
      `/boards/${boardId}/items`,
      body({ kind: "note", asset_id: "", ...note, rotation: 0, z: 0 }),
    ),
  // Sends a list-view selection to a board with a server-assigned default
  // layout (issue #76). `added` may be < asset_ids.length because assets already
  // on the board are skipped.
  batchAddToBoard: (id: string, asset_ids: string[]) =>
    req<{ added: number }>(`/boards/${id}/items/batch`, body({ asset_ids })),
  updateBoardItems: (id: string, items: BoardItem[]) =>
    req<{ updated: number }>(`/boards/${id}/items`, patch({ items })),
  deleteBoardItem: (id: string, itemId: string) =>
    req<{ deleted: boolean }>(`/boards/${id}/items/${itemId}`, {
      method: "DELETE",
    }),

  // Collections (issue #55): a manually-curated, nestable asset group. Cover is
  // resolved server-side (explicit value, else the ord-smallest member, else "");
  // PATCH cover to a member asset id, or "" to clear back to that fallback.
  listCollections: () => req<Collection[]>("/collections"),
  createCollection: (c: NewCollection) => req<Collection>("/collections", body(c)),
  updateCollection: (id: string, data: CollectionPatch) =>
    req<{ ok: boolean }>(`/collections/${id}`, patch(data)),
  deleteCollection: (id: string) =>
    req<{ deleted: boolean }>(`/collections/${id}`, { method: "DELETE" }),

  listCollectionItems: (id: string) => req<CollectionItem[]>(`/collections/${id}/items`),
  addCollectionItem: (id: string, asset_id: string) =>
    req<{ added: boolean }>(`/collections/${id}/items`, body({ asset_id })),
  removeCollectionItem: (id: string, assetId: string) =>
    req<{ deleted: boolean }>(`/collections/${id}/items/${assetId}`, { method: "DELETE" }),
  // asset_ids must be the complete, reordered member list (the server 400s on a
  // set mismatch), so the collection view always sends every member's id.
  reorderCollectionItems: (id: string, asset_ids: string[]) =>
    req<{ reordered: number }>(`/collections/${id}/items/order`, {
      method: "PUT",
      body: JSON.stringify({ asset_ids }),
    }),

  listMissing: (limit = 200, offset = 0) =>
    req<Asset[]>(`/assets?status=missing&limit=${limit}&offset=${offset}`),
  relocate: (id: string, new_path: string, provider?: string) =>
    req<{ relocated: string }>(
      `/assets/${id}/relocate`,
      body({ new_path, ...(provider ? { provider } : {}) }),
    ),
   rebase: (old_prefix: string, new_prefix: string, provider = "local") =>
     req<{ rebased: boolean }>(
       "/relocate/rebase",
       body({ old_prefix, new_prefix, provider }),
     ),

   uploadThumb: async (id: string, blob: Blob): Promise<void> => {
     const form = new FormData();
     form.append("file", blob, "thumb.png");
     const res = await fetch(`${BASE}/assets/${id}/thumb`, {
       method: "POST",
       body: form,
     });
     if (!res.ok) {
       let msg = `${res.status} ${res.statusText}`;
       try {
         const body = (await res.json()) as { error?: string };
         if (body.error) msg = body.error;
       } catch { /* non-JSON error body */ }
       throw new Error(msg);
     }
   },

  // Imports one file into the library via multipart POST /import: the backend
  // copies it in, indexes, thumbnails, and dedupes (issue #18 pipeline). The
  // `filename` carries the extension so the server infers kind/ext — essential
  // for nameless clipboard blobs, which the caller names pasted-<ts>.<ext>
  // (issue #89). destSubdir is a filesystem subdir, NOT the virtual folder_id.
  importAsset: async (file: Blob, filename: string, destSubdir?: string): Promise<ImportResult> => {
    const form = new FormData();
    form.append("file", file, filename);
    if (destSubdir) form.append("dest_subdir", destSubdir);
    const res = await fetch(`${BASE}/import`, { method: "POST", body: form });
    if (!res.ok) {
      let msg = `${res.status} ${res.statusText}`;
      try {
        const body = (await res.json()) as { error?: string };
        if (body.error) msg = body.error;
      } catch { /* non-JSON error body */ }
      throw new Error(msg);
    }
    return (await res.json()) as ImportResult;
  },
};
