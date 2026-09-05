// Typed client for the DAM HTTP API. All calls target `/api/dam/*`; the Vite
// dev server proxies to the Go backend on :8080, and in production the same
// origin serves both the SPA and the API.
import type {
  Asset,
  ColorMatch,
  Folder,
  NewFolder,
  NewTag,
  Tag,
} from "../types";

const BASE = "/api/dam";

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

export const thumbUrl = (id: string): string => `${BASE}/assets/${id}/thumb`;

export const api = {
  listAssets: (limit = 200, offset = 0) =>
    req<Asset[]>(`/assets?limit=${limit}&offset=${offset}`),

  searchKeyword: (q: string, limit = 200, offset = 0) =>
    req<Asset[]>(
      `/search?q=${encodeURIComponent(q)}&limit=${limit}&offset=${offset}`,
    ),

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
};
