import { withBasePath } from "../lib/base";

let cached: string | null = null;
let inflight: Promise<string> | null = null;

function hex(buf: ArrayBuffer): string {
  return Array.from(new Uint8Array(buf), (b) => b.toString(16).padStart(2, "0")).join("");
}

/** Finds a nonce so sha256(challenge:nonce) starts with `difficulty` zeros. */
async function solvePow(challenge: string, difficulty: number): Promise<string> {
  const prefix = "0".repeat(difficulty);
  for (let i = 0; ; i++) {
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(`${challenge}:${i}`));
    if (hex(digest).startsWith(prefix)) return String(i);
  }
}

async function fetchCapability(): Promise<string> {
  const res = await fetch(withBasePath("/api/capability"));
  if (!res.ok) throw new Error("capability unavailable");
  const data = (await res.json()) as { capability?: string; challenge?: string; difficulty?: number };

  if (data.challenge && data.difficulty) {
    const nonce = await solvePow(data.challenge, data.difficulty);
    const verify = await fetch(withBasePath("/api/capability"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ challenge: data.challenge, nonce }),
    });
    if (!verify.ok) throw new Error("proof of work rejected");
    const solved = (await verify.json()) as { capability: string };
    cached = solved.capability;
    return cached;
  }
  cached = data.capability ?? "";
  return cached;
}

/** Returns a valid capability token, minting one if needed. Parallel
 * callers share a single in-flight request. */
export async function ensureCapability(): Promise<string> {
  if (cached) return cached;
  if (!inflight) {
    inflight = fetchCapability().finally(() => {
      inflight = null;
    });
  }
  return inflight;
}

/** Drops the cached token after a rejection so the next call re-mints. */
export function clearCapability(): void {
  cached = null;
}

/** Fetch wrapper for mutations: attaches the capability and refreshes once on 403. */
export async function mutate(url: string, init: RequestInit = {}): Promise<Response> {
  const attempt = async (): Promise<Response> => {
    const token = await ensureCapability();
    const headers = new Headers(init.headers);
    if (token) headers.set("X-Capability", token);
    return fetch(url, { ...init, headers });
  };

  let res = await attempt();
  if (res.status === 403) {
    clearCapability();
    res = await attempt();
  }
  return res;
}
