import { z } from "zod";
import { withBasePath } from "../lib/base";

const SystemDynamicSettingsSchema = z.object({
  max_upload: z.number(),
  max_text_size: z.number(),
  guard: z.boolean(),
  request_rate: z.number(),
  download_rate: z.number(),
  pow_difficulty: z.number(),
  trusted_proxies: z.string(),
});
export type SystemDynamicSettings = z.infer<typeof SystemDynamicSettingsSchema>;

const SystemInfoSchema = z.object({
  root: z.string(),
  database: z.string(),
  cache_dir: z.string(),
  base_url: z.string(),
  address: z.string(),
  version: z.string(),
  commit: z.string(),
  os: z.string(),
  arch: z.string(),
  go_version: z.string(),
  uptime_seconds: z.number(),
});

const SystemSettingsResponseSchema = z.object({
  dynamic: SystemDynamicSettingsSchema,
  info: SystemInfoSchema,
});
export type SystemSettingsResponse = z.infer<typeof SystemSettingsResponseSchema>;

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
    const raw = await res.json();
    return SystemSettingsResponseSchema.parse(raw);
  },

  update: async (data: UpdateSystemSettingsRequest): Promise<SystemSettingsResponse> => {
    const res = await fetch(withBasePath("/api/settings/system"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", Origin: window.location.origin },
      body: JSON.stringify(data),
    });
    if (!res.ok) throw new Error(await jsonError(res));
    const raw = await res.json();
    return SystemSettingsResponseSchema.parse(raw);
  },
};
