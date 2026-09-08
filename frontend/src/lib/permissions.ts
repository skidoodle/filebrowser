import type { Me } from "../api/auth";

/**
 * Reports whether path lies at or below the user's scope prefix.
 * An empty scope means the whole root.
 */
export function inScope(scope: string | undefined, path: string): boolean {
  if (!scope) return true;
  const rel = path === "." || path === "" ? "." : path;
  return rel === scope || rel.startsWith(scope + "/");
}

/**
 * Write permission targeting a specific path: admin anywhere, scoped users
 * strictly inside their scope. The scope folder itself is off-limits —
 * it cannot be deleted, renamed, replaced or toggled private by its owner.
 */
export function canWritePath(me: Me | undefined, path: string): boolean {
  if (!me) return false;
  if (me.insecure || me.admin) return true;
  if (!me.username) return false; // anonymous guests never write
  const scope = me.scope ?? "";
  if (!inScope(scope, path)) return false;
  const rel = path === "." || path === "" ? "." : path;
  return rel !== scope; // the scope folder itself is protected
}

/**
 * Write permission for creating things INSIDE a directory: the scope
 * folder itself may be populated (its children are strictly inside),
 * it just can never be the target of a mutation itself.
 */
export function canWriteIn(me: Me | undefined, dir: string): boolean {
  if (!me) return false;
  if (me.insecure || me.admin) return true;
  if (!me.username) return false;
  return inScope(me.scope, dir);
}
