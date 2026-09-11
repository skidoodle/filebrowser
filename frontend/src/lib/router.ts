import { useSyncExternalStore } from "react";
import { withBasePath, withoutBasePath } from "./base";
import { matchRoute, parseRoute, routePath as canonicalRoutePath, type AppRoute } from "./routes";

export { parseRoute } from "./routes";
export type { AppRoute, CreateKind, SettingsTab } from "./routes";

import { z } from "zod";

interface NavigateOptions {
  replace?: boolean;
}

const HISTORY_STATE_KEY = "filebrowserRoute";
const listeners = new Set<() => void>();

const RouteHistoryStateSchema = z.object({
  dir: z.string().optional(),
  edit: z.boolean().optional(),
});

function readStoredState(state: unknown): { dir?: string; edit?: boolean } {
  if (typeof state !== "object" || state === null) return {};
  const routeState = (state as Record<string, unknown>)[HISTORY_STATE_KEY];
  const parsed = RouteHistoryStateSchema.safeParse(routeState);
  return parsed.success ? parsed.data : {};
}

function historyState(route: AppRoute): Record<string, unknown> {
  const current = typeof window.history.state === "object" && window.history.state !== null ? window.history.state : {};
  const stored: Record<string, unknown> = { dir: route.dir };
  if (route.page === "viewer" && route.edit) {
    stored.edit = true;
  }
  return { ...current, [HISTORY_STATE_KEY]: stored };
}

function readRoute(): AppRoute {
  const pathname = withoutBasePath(window.location.pathname) ?? "/";
  const stored = readStoredState(window.history.state);
  return parseRoute(pathname, stored.dir, stored.edit ? true : undefined);
}

let snapshot = readRoute();

// Non-canonical locations (stray query strings, trailing slashes) are
// immediately rewritten to the canonical URL.
const initialPath = routePath(snapshot);
if (`${window.location.pathname}${window.location.search}` !== initialPath) {
  window.history.replaceState(historyState(snapshot), "", initialPath);
}

function emit() {
  snapshot = readRoute();
  for (const listener of listeners) listener();
}

window.addEventListener("popstate", emit);

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Returns the current route without subscribing to future navigation. */
export function currentRoute(): AppRoute {
  return snapshot;
}

/** Subscribes a component to the current typed application route. */
export function useRoute(): AppRoute {
  return useSyncExternalStore(subscribe, () => snapshot, () => snapshot);
}

/** Returns the canonical URL, including the configured application mount path. */
export function routePath(route: AppRoute): string {
  return withBasePath(canonicalRoutePath(route));
}

/** Navigates to a route while retaining its background directory in history. */
export function navigate(route: AppRoute, options: NavigateOptions = {}): void {
  const method = options.replace ? "replaceState" : "pushState";
  window.history[method](historyState(route), "", routePath(route));
  emit();
}

/**
 * Navigates an internal anchor href. Returns false when the href is external or
 * does not match an application route, allowing the browser to handle it.
 */
export function navigateHref(href: string, options: NavigateOptions = {}): boolean {
  const url = new URL(href, window.location.href);
  if (url.origin !== window.location.origin) return false;

  const pathname = withoutBasePath(url.pathname);
  if (pathname === null) return false;

  const route = matchRoute(pathname, snapshot.dir);
  if (route === null) return false;

  navigate(route, options);
  return true;
}
