/** Joins a storage directory and a name into a slash-separated path. */
export function joinPath(dir: string, name: string): string {
  if (dir === "" || dir === "." || dir === "/") return name;
  return `${dir.replace(/\/+$/, "")}/${name}`;
}

/** Parent directory of a storage path ("." for root-level entries). */
export function parentPath(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx === -1 ? "." : path.slice(0, idx);
}
