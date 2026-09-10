import { withBasePath } from "../lib/base";

export interface SystemDynamicSettings {
  max_upload: number; // bytes
  max_text_size: number; // bytes
  guard: boolean;
  request_rate: number; // req/s
  download_rate: number; // bytes/s
  pow_difficulty: number;
  trusted_proxies: string;
}

export interface SystemInfo {
  root: string;
  database: string;
  cache_dir: string;
  base_url: string;
  address: string;
  version: string;
  commit: string;
  os: string;
  arch: string;
  go_version: string;
  uptime_seconds: number;
}

export interface SystemSettingsResponse {
  dynamic: SystemDynamicSettings;
  info: SystemInfo;
}

export interface UpdateSystemSettingsRequest {
  max_upload?: number;
  max_text_size?: number;
  guard?: boolean;
  request_rate?: number;
  download_rate?: number;
  pow_difficulty?: number;
  trusted_proxies?: string;
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

export const system = {
  get: async (): Promise<SystemSettingsResponse> => {
    const res = await fetch(withBasePath("/api/settings/system"), {
      headers: { Origin: window.location.origin },
    });
    if (!res.ok) throw new Error(await jsonError(res));
    return res.json() as Promise<SystemSettingsResponse>;
  },

  update: async (data: UpdateSystemSettingsRequest): Promise<SystemSettingsResponse> => {
    const res = await fetch(withBasePath("/api/settings/system"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify(data),
    });
    if (!res.ok) throw new Error(await jsonError(res));
    return res.json() as Promise<SystemSettingsResponse>;
  },
};
