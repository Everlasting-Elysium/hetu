// Typed client for the DAM HTTP API. All calls target `/api/dam/*`; the Vite
// dev server proxies to the Go backend on :8080, and in production the same
// origin serves both the SPA and the API.
import type {
  Asset,
  AssetKind,
  Board,
  BoardItem,
  ColorMatch,
  Facets,
  Folder,
  NewFolder,
  NewTag,
  Query,
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
  assetTags: (id: string) => req<Tag[]>(`/assets/${id}/tags`),

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
};
