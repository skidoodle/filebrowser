const editTokens = new Map<string, string>();

export function setEditToken(path: string, token: string) {
  editTokens.set(path, token);
}

export function getEditToken(path: string): string | undefined {
  return editTokens.get(path);
}

export function hasEditToken(path: string): boolean {
  return editTokens.has(path);
}

export function clearEditToken(path: string) {
  editTokens.delete(path);
}
