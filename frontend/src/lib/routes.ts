export type SettingsTab = "profile" | "users" | "policy" | "system" | "about";
type CreateKind = "dir" | "file";

export type AppRoute =
  | { page: "files"; dir: string }
  | { page: "viewer"; path: string; dir: string; edit?: boolean }
  | { page: "create"; kind: CreateKind; dir: string }
  | { page: "settings"; tab: SettingsTab; dir: string }
  | { page: "auth"; mode: "login" | "setup"; dir: string };

/**
 * First path segments claimed by application routes. Root-level entries
 * with these names could never be browsed: the router reads /view, /new,
 * /settings, /login and /setup as pages, not storage paths. The backend
 * rejects writes into them as well (internal/api/reserved.go).
 */
const RESERVED_ROOTS = new Set(["view", "new", "settings", "login", "setup"]);

/** Reports whether a storage path starts with a route-claimed segment. */
export function isReservedPath(path: string): boolean {
  const [first = ""] = path.split("/");
  return RESERVED_ROOTS.has(first);
}

function normalizeStoragePath(path: string): string {
  const segments: string[] = [];
  for (const segment of path.split("/")) {
    if (segment === "" || segment === ".") continue;
    if (segment === "..") {
      segments.pop();
      continue;
    }
    segments.push(segment);
  }
  return segments.join("/") || ".";
}

function encodeStoragePath(path: string): string {
  const normalized = normalizeStoragePath(path);
  if (normalized === ".") return "";
  return normalized.split("/").map(encodeURIComponent).join("/");
}

function decodeStoragePath(segments: string[]): string | null {
  try {
    const decoded = segments.map((segment) => decodeURIComponent(segment));
    if (decoded.some((segment) => segment.includes("/"))) return null;
    return normalizeStoragePath(decoded.join("/"));
  } catch {
    return null;
  }
}

function parentDirectory(path: string): string {
  const normalized = normalizeStoragePath(path);
  const separator = normalized.lastIndexOf("/");
  return separator === -1 ? "." : normalized.slice(0, separator);
}

/** Returns the canonical, query-free URL for an application route. */
export function routePath(route: AppRoute): string {
  switch (route.page) {
    case "files": {
      const path = encodeStoragePath(route.dir);
      return path ? `/${path}` : "/";
    }
    case "viewer":
      return `/view/${encodeStoragePath(route.path)}`;
    case "create": {
      const kind = route.kind === "dir" ? "folder" : "file";
      const dir = encodeStoragePath(route.dir);
      return dir ? `/new/${kind}/${dir}` : `/new/${kind}`;
    }
    case "settings":
      return `/settings/${route.tab}`;
    case "auth":
      return `/${route.mode}`;
  }
}

function routeFromPath(pathname: string, backgroundDir?: string, backgroundEdit?: boolean): AppRoute | null {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length === 0) return { page: "files", dir: "." };

  switch (segments[0]) {
    case "view": {
      if (segments.length < 2) return null;
      const path = decodeStoragePath(segments.slice(1));
      if (path === null || path === ".") return null;
      const route: AppRoute = { page: "viewer", path, dir: backgroundDir ?? parentDirectory(path) };
      if (backgroundEdit) route.edit = true;
      return route;
    }
    case "new": {
      const kind = segments[1] === "folder" ? "dir" : segments[1] === "file" ? "file" : null;
      if (kind === null) return null;
      const dir = decodeStoragePath(segments.slice(2));
      return dir === null ? null : { page: "create", kind, dir };
    }
    case "settings": {
      if (segments.length > 2) return null;
      const tab = segments[1] ?? "profile";
      if (tab !== "profile" && tab !== "users" && tab !== "policy" && tab !== "system" && tab !== "about") return null;
      return { page: "settings", tab, dir: backgroundDir ?? "." };
    }
    case "login":
    case "setup":
      if (segments.length !== 1) return null;
      return { page: "auth", mode: segments[0], dir: backgroundDir ?? "." };
    default: {
      // Every other path is a directory. Root-level folders named after a
      // route above (view, new, settings, login, setup) are shadowed by it.
      const dir = decodeStoragePath(segments);
      return dir === null ? null : { page: "files", dir };
    }
  }
}

/** Returns null when a location is not an application route. */
export function matchRoute(pathname: string, backgroundDir?: string, backgroundEdit?: boolean): AppRoute | null {
  return routeFromPath(pathname, backgroundDir, backgroundEdit);
}

/** Parses a location, falling back to the root browser for malformed input. */
export function parseRoute(pathname: string, backgroundDir?: string, backgroundEdit?: boolean): AppRoute {
  return matchRoute(pathname, backgroundDir, backgroundEdit) ?? { page: "files", dir: "." };
}
