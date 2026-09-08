import { mutate } from "./capability";
import { withBasePath } from "../lib/base";
import { currentRoute, navigate } from "../lib/router";
import type { FileMeta, FileInfo, Health, Listing, Usage } from "../types";

function adminInit(init: RequestInit = {}): RequestInit {
  const headers = new Headers(init.headers);
  headers.set("Origin", window.location.origin);
  return { ...init, headers };
}

async function adminError(res: Response): Promise<never> {
  let msg = res.statusText;
  try {
    const body = (await res.json()) as { error?: string };
    if (body?.error) msg = body.error;
  } catch {
    // non-json error body
  }
  if (res.status === 401) {
    navigate({ page: "auth", mode: "login", dir: currentRoute().dir });
  }
  throw new Error(msg);
}

async function errorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (body?.error) return body.error;
  } catch {
    // non-json error body
  }
  return res.statusText;
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new Error(await errorMessage(res));
  }
  return res.json() as Promise<T>;
}

export type SortBy = "name" | "size" | "modified";
export type SortOrder = "asc" | "desc";

export const api = {
  health: () => fetch(withBasePath("/api/health")).then((r) => json<Health>(r)),

  list: (path: string, sort: SortBy = "name", order: SortOrder = "asc") =>
    fetch(withBasePath(`/api/list?path=${encodeURIComponent(path)}&sort=${sort}&order=${order}`)).then((r) =>
      json<Listing>(r),
    ),

  meta: (path: string, checksum?: string) =>
    fetch(
      withBasePath(`/api/meta?path=${encodeURIComponent(path)}${checksum ? `&checksum=${checksum}` : ""}`),
    ).then((r) => json<FileMeta>(r)),

  usage: () => fetch(withBasePath("/api/usage")).then((r) => json<Usage>(r)),

  search: async (q: string, limit = 50): Promise<FileInfo[]> => {
    const res = await fetch(withBasePath(`/api/search?q=${encodeURIComponent(q)}&limit=${limit}`));
    if (!res.ok) throw new Error(await errorMessage(res));
    const text = await res.text();
    return text
      .split("\n")
      .filter((line) => line.trim().length > 0)
      .map((line) => JSON.parse(line) as FileInfo);
  },

  createDir: (path: string) =>
    mutate(withBasePath("/api/dir"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    }).then((r) => json<FileInfo>(r)),

  createFile: (path: string) =>
    mutate(withBasePath("/api/file"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    }).then((r) => json<FileInfo>(r)),

  rawUrl: (path: string, inline = false) =>
    withBasePath(`/api/raw?path=${encodeURIComponent(path)}${inline ? "&inline=true" : ""}`),

  downloadUrl: (path: string, files?: string[], algo: "zip" | "tar.gz" = "zip") => {
    const params = new URLSearchParams({ path, algo });
    if (files && files.length > 0) params.set("files", files.join(","));
    return withBasePath(`/api/download?${params.toString()}`);
  },

  /** 256px square thumbnail for grids. */
  thumbUrl: (path: string) => withBasePath(`/api/thumb?path=${encodeURIComponent(path)}`),

  /** 1080px-fit preview for lightbox / info panel. */
  bigUrl: (path: string) => withBasePath(`/api/big?path=${encodeURIComponent(path)}`),

  /** Delete files or directories (recursively). Admin-only. */
  delete: async (paths: string[]): Promise<void> => {
    const res = await fetch(withBasePath("/api/delete"), adminInit({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ paths }),
    }));
    if (!res.ok) await adminError(res);
  },

  /** Rename or relocate a single entry. Admin-only. */
  move: async (from: string, to: string): Promise<FileInfo> => {
    const res = await fetch(withBasePath("/api/move"), adminInit({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ from, to }),
    }));
    if (!res.ok) await adminError(res);
    return res.json() as Promise<FileInfo>;
  },

  /** Overwrite or create a file with the given content. Admin-only. */
  save: async (path: string, content: string): Promise<FileInfo> => {
    const res = await fetch(withBasePath(`/api/raw?path=${encodeURIComponent(path)}`), adminInit({
      method: "PUT",
      headers: { "Content-Type": "text/plain; charset=utf-8" },
      body: content,
    }));
    if (!res.ok) await adminError(res);
    return res.json() as Promise<FileInfo>;
  },
};
