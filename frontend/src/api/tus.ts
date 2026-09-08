import { defaultOptions, Upload } from "tus-js-client";
import type { DetailedError } from "tus-js-client";
import { clearCapability, ensureCapability } from "./capability";
import { withBasePath } from "../lib/base";

function errorStatus(err: Error): number {
  const detailed = err as DetailedError;
  return detailed.originalResponse?.getStatus() ?? 0;
}

/**
 * Friendly message for an upload failure.
 */
export function tusErrorText(err: Error): string {
  const detailed = err as DetailedError;
  let body = "";
  try {
    body = detailed.originalResponse?.getBody() ?? "";
  } catch {
    // body unavailable
  }
  try {
    const parsed = JSON.parse(body) as { error?: string };
    if (parsed.error) return parsed.error;
  } catch {
    // non-json error body
  }
  return err.message
    .replace(/^tus:\s*/, "")
    .split(", originated from request")[0];
}

export const UPLOAD_CHUNK_SIZE = 16 * 1024 * 1024;

export interface UploadCallbacks {
  onProgress?: (uploaded: number, total: number) => void;
  onSuccess?: () => void;
  onError?: (err: Error) => void;
  onCapabilityRejected?: () => void;
}

/**
 * Removes stored fingerprints for a target path. Called after a failed
 * upload so a retry starts fresh instead of patching a dead URL (e.g.
 * after a server restart dropped the in-memory session).
 */
async function purgePreviousUploads(targetPath: string): Promise<void> {
  try {
    const storage = defaultOptions.urlStorage;
    const all = await storage.findAllUploads();
    await Promise.all(
      all
        .filter((p) => p.metadata?.path === targetPath)
        .map((p) => storage.removeUpload(p.urlStorageKey)),
    );
  } catch {
    // Storage unavailable; nothing to purge.
  }
}

/**
 * createUpload builds a tus upload targeting /api/tus and starts it,
 * transparently resuming a previous attempt of the same file to the same
 * path when one exists. The destination path travels in Upload-Metadata
 * ("path"), matching the Go handler.
 */
export function createUpload(file: File, targetPath: string, cb: UploadCallbacks): Upload {
  const handle = new Upload(file, {
    endpoint: withBasePath("/api/tus"),
    retryDelays: [0, 1000, 3000, 5000, 15000, 30000],
    chunkSize: UPLOAD_CHUNK_SIZE,
    removeFingerprintOnSuccess: true,
    headers: {} as Record<string, string>,
    metadata: {
      filename: file.name,
      filetype: file.type || "application/octet-stream",
      path: targetPath,
    },
    onProgress: cb.onProgress,
    onSuccess: cb.onSuccess,
    onShouldRetry: (err) => {
      const detailed = err as DetailedError;
      return !(errorStatus(err) === 409 && detailed.originalRequest?.getMethod() === "POST");
    },
    onError: (err) => {
      if (errorStatus(err) === 403 && cb.onCapabilityRejected) {
        clearCapability();
        void purgePreviousUploads(targetPath);
        cb.onCapabilityRejected();
        return;
      }
      cb.onError?.(err);
    },
  });

  void (async () => {
    try {
      // tus sends this header on every request (create/patch/head).
      const token = await ensureCapability();
      if (token) handle.options.headers = { "X-Capability": token };

      const previous = await handle.findPreviousUploads();
      const match = previous.find(
        (p) =>
          p.metadata?.path === targetPath &&
          (p.uploadUrl !== null || (p.parallelUploadUrls?.length ?? 0) > 0),
      );
      if (match?.uploadUrl) {
        handle.resumeFromPreviousUpload(match);
      }
    } catch {
      // No URL storage or capability problem; start from scratch.
    }
    handle.start();
  })();

  return handle;
}
