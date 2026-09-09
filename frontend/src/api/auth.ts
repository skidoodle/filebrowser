import { withBasePath } from "../lib/base";

export type AccessPolicy = "public" | "readonly" | "private";

export interface Me {
  admin: boolean;
  insecure: boolean;
  initialized: boolean;
  username?: string;
  scope?: string;
  access_policy?: AccessPolicy;
}

export interface User {
  id: number;
  username: string;
  admin: boolean;
  scope: string;
  isOriginal: boolean;
}

async function jsonError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (body?.error) return body.error;
  } catch {
    // non-json error body
  }
  return res.statusText;
}

async function expectNoContent(res: Response): Promise<void> {
  if (res.status === 401) throw new Error("Invalid credentials");
  if (res.status === 409) throw new Error("Account already exists");
  if (res.status === 400) throw new Error(await jsonError(res));
  if (res.status === 429) throw new Error("Too many failed attempts, wait a moment");
  if (!res.ok) throw new Error(await jsonError(res));
}

export const auth = {
  me: () => fetch(withBasePath("/api/me")).then((r) => {
    if (!r.ok) throw new Error(r.statusText);
    return r.json() as Promise<Me>;
  }),

  setup: (password: string) =>
    fetch(withBasePath("/api/auth/setup"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    }).then(expectNoContent),

  login: (username: string, password: string) =>
    fetch(withBasePath("/api/auth/login"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    }).then(expectNoContent),

  logout: () => fetch(withBasePath("/api/auth/logout"), { method: "POST" }).then(() => undefined),

  changeUsername: async (username: string): Promise<void> => {
    const res = await fetch(withBasePath("/api/auth/password"), {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify({ username }),
    });
    if (!res.ok) throw new Error(await jsonError(res));
  },

  changePassword: async (current: string, password: string): Promise<void> => {
    const res = await fetch(withBasePath("/api/auth/password"), {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify({ current, password }),
    });
    if (!res.ok) {
      const msg = res.status === 403 ? "Current password is wrong" : await jsonError(res);
      throw new Error(msg);
    }
  },

  setPrivate: async (path: string): Promise<void> => {
    const res = await fetch(withBasePath("/api/private"), {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify({ path }),
    });
    if (!res.ok) throw new Error(await jsonError(res));
  },

  unsetPrivate: async (path: string): Promise<void> => {
    const res = await fetch(withBasePath(`/api/private?path=${encodeURIComponent(path)}`), {
      method: "DELETE",
      headers: { Origin: window.location.origin },
    });
    if (!res.ok) throw new Error(await jsonError(res));
  },

  getAccessPolicy: async (): Promise<AccessPolicy> => {
    const res = await fetch(withBasePath("/api/settings/policy"), {
      headers: { Origin: window.location.origin },
    });
    if (!res.ok) throw new Error(await jsonError(res));
    const data = (await res.json()) as { access_policy: AccessPolicy };
    return data.access_policy;
  },

  setAccessPolicy: async (policy: AccessPolicy): Promise<void> => {
    const res = await fetch(withBasePath("/api/settings/policy"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify({ access_policy: policy }),
    });
    if (!res.ok) throw new Error(await jsonError(res));
  },
};

export interface NewUser {
  username: string;
  password: string;
  admin: boolean;
  scope: string;
}

export interface User {
  id: number;
  username: string;
  admin: boolean;
  scope: string;
  isOriginal: boolean;
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) throw new Error(await jsonError(res));
  return res.json() as Promise<T>;
}

export const users = {
  list: () => fetch(withBasePath("/api/users")).then((r) => json<User[]>(r)),

  create: async (u: NewUser): Promise<void> => {
    const res = await fetch(withBasePath("/api/users"), {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify(u),
    });
    if (!res.ok) throw new Error(await jsonError(res));
  },

  update: async (id: number, patch: { username?: string; admin?: boolean; scope?: string; password?: string }): Promise<void> => {
    const res = await fetch(withBasePath(`/api/users/${id}`), {
      method: "PATCH",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify(patch),
    });
    if (!res.ok) {
      const msg = await jsonError(res);
      throw new Error(msg);
    }
  },

  remove: async (id: number): Promise<void> => {
    const res = await fetch(withBasePath(`/api/users/${id}`), {
      method: "DELETE",
      headers: { Origin: window.location.origin },
    });
    if (!res.ok) {
      const msg = await jsonError(res);
      throw new Error(msg);
    }
  },
};
