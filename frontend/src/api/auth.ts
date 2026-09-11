import { z } from "zod";
import { withBasePath } from "../lib/base";
import {
  AccessPolicySchema,
  MeSchema,
  NewUserSchema,
  UserSchema,
  type AccessPolicy,
  type Me,
  type NewUser,
  type User,
} from "../types";

export type { AccessPolicy, Me, NewUser, User };
export { AccessPolicySchema, MeSchema, NewUserSchema, UserSchema };

async function jsonError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (body?.error) return body.error;
  } catch {
    // non-json error body
  }
  return res.statusText;
}

async function parseJson<T>(res: Response, schema: z.ZodType<T>): Promise<T> {
  if (!res.ok) throw new Error(await jsonError(res));
  const raw = await res.json();
  return schema.parse(raw);
}

async function expectNoContent(res: Response): Promise<void> {
  if (res.status === 401) throw new Error("Invalid credentials");
  if (res.status === 409) throw new Error("Account already exists");
  if (res.status === 400) throw new Error(await jsonError(res));
  if (res.status === 429) throw new Error("Too many failed attempts, wait a moment");
  if (!res.ok) throw new Error(await jsonError(res));
}

export const auth = {
  me: () => fetch(withBasePath("/api/me")).then((r) => parseJson(r, MeSchema)),

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
    const data = await parseJson(res, z.object({ access_policy: AccessPolicySchema }));
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

export const users = {
  list: () => fetch(withBasePath("/api/users")).then((r) => parseJson(r, z.array(UserSchema))),

  create: async (u: NewUser): Promise<void> => {
    const payload = NewUserSchema.parse(u);
    const res = await fetch(withBasePath("/api/users"), {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify(payload),
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
