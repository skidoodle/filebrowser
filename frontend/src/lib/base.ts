function normalizeBasePath(path: string): string {
  if (path === "" || path === "/") return "";
  const prefixed = path.startsWith("/") ? path : `/${path}`;
  return prefixed.replace(/\/+$/, "");
}

const meta =
  typeof document === "undefined"
    ? null
    : document.querySelector<HTMLMetaElement>('meta[name="filebrowser-base"]');
const basePath = normalizeBasePath(meta?.content ?? "");

/** Prefixes an absolute application path with a normalized mount path. */
export function addBasePath(base: string, pathname: string): string {
  if (!pathname.startsWith("/")) throw new Error(`path must be absolute: ${pathname}`);
  const normalizedBase = normalizeBasePath(base);
  if (normalizedBase === "") return pathname;
  return pathname === "/" ? `${normalizedBase}/` : `${normalizedBase}${pathname}`;
}

/** Removes a mount path, or rejects a pathname outside it. */
export function removeBasePath(base: string, pathname: string): string | null {
  const normalizedBase = normalizeBasePath(base);
  if (normalizedBase === "") return pathname;
  if (pathname === normalizedBase || pathname === `${normalizedBase}/`) return "/";
  if (!pathname.startsWith(`${normalizedBase}/`)) return null;
  return pathname.slice(normalizedBase.length);
}

/** Prefixes an absolute application path with the configured mount path. */
export function withBasePath(pathname: string): string {
  return addBasePath(basePath, pathname);
}

/** Removes the configured mount path, or rejects a path outside the app. */
export function withoutBasePath(pathname: string): string | null {
  return removeBasePath(basePath, pathname);
}
