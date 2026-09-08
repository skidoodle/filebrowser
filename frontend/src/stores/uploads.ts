import type { Upload } from "tus-js-client";
import { create } from "zustand";
import { createUpload, tusErrorText } from "../api/tus";
import { queryClient } from "../lib/queryClient";
import { joinPath } from "../lib/path";

export interface UploadItem {
  id: string;
  name: string;
  path: string;
  file: File;
  size: number;
  uploaded: number;
  status: "queued" | "active" | "done" | "error";
  error?: string;
  capabilityRetries?: number;
  speed?: number;
}

interface UploadsState {
  items: UploadItem[];
  enqueue: (baseDir: string, files: File[]) => void;
  retry: (id: string) => void;
  remove: (id: string) => void;
  clearFinished: () => void;
}

const MAX_CONCURRENT = 3;
const handles = new Map<string, Upload>();
let nextId = 1;
const samples = new Map<string, { t: number; b: number }[]>();
const SPEED_WINDOW_MS = 5000;

function recordProgress(id: string, uploaded: number): number | undefined {
  const now = Date.now();
  const recent = (samples.get(id) ?? []).filter((s) => now - s.t <= SPEED_WINDOW_MS);
  recent.push({ t: now, b: uploaded });
  samples.set(id, recent);
  const dt = (now - recent[0].t) / 1000;
  if (recent.length < 2 || dt < 0.5) return undefined;
  return (uploaded - recent[0].b) / dt;
}

function resetSamples(id: string) {
  samples.delete(id);
}

function relativePath(file: File): string {
  const rel = (file as File & { webkitRelativePath?: string }).webkitRelativePath;
  return rel && rel.length > 0 ? rel : file.name;
}

function updateItem(id: string, patch: Partial<UploadItem>) {
  useUploads.setState((state) => ({
    items: state.items.map((it) => (it.id === id ? { ...it, ...patch } : it)),
  }));
}

function startItem(item: UploadItem) {
  updateItem(item.id, { status: "active", error: undefined });
  const handle = createUpload(item.file, item.path, {
    onProgress: (uploaded) => {
      const speed = recordProgress(item.id, uploaded);
      updateItem(item.id, speed === undefined ? { uploaded } : { uploaded, speed });
    },
    onSuccess: () => {
      resetSamples(item.id);
      handles.delete(item.id);
      updateItem(item.id, { status: "done", uploaded: item.size, speed: undefined });
      void queryClient.invalidateQueries({ queryKey: ["list"] });
      void queryClient.invalidateQueries({ queryKey: ["usage"] });
      pump();
    },
    onError: (err) => {
      resetSamples(item.id);
      handles.delete(item.id);
      updateItem(item.id, { status: "error", error: tusErrorText(err), speed: undefined });
      pump();
    },
    onCapabilityRejected: () => {
      resetSamples(item.id);
      handles.delete(item.id);
      const current = useUploads
        .getState()
        .items.find((it) => it.id === item.id);
      const retries = (current?.capabilityRetries ?? 0) + 1;
      if (retries > 2) {
        updateItem(item.id, { status: "error", error: "authorization rejected by server", speed: undefined });
        pump();
        return;
      }
      updateItem(item.id, { status: "queued", uploaded: 0, speed: undefined, capabilityRetries: retries });
      pump();
    },
  });
  handles.set(item.id, handle);
}

export const useUploads = create<UploadsState>()((set) => ({
  items: [],

  enqueue: (baseDir, files) => {
    const items: UploadItem[] = files.map((file) => ({
      id: `up-${nextId++}`,
      name: file.name,
      path: joinPath(baseDir, relativePath(file)),
      file,
      size: file.size,
      uploaded: 0,
      status: "queued",
    }));
    set((state) => ({ items: [...state.items, ...items] }));
    pump();
  },

  retry: (id) => {
    resetSamples(id);
    handles.delete(id);
    updateItem(id, { status: "queued", uploaded: 0, speed: undefined, error: undefined });
    pump();
  },

  remove: (id) => {
    resetSamples(id);
    const handle = handles.get(id);
    handles.delete(id);
    void handle?.abort(true).catch(() => { });
    set((state) => ({ items: state.items.filter((it) => it.id !== id) }));
    pump();
  },

  clearFinished: () => {
    set((state) => ({
      items: state.items.filter((it) => it.status === "queued" || it.status === "active"),
    }));
  },
}));

function pump() {
  const { items } = useUploads.getState();
  const active = items.filter((it) => it.status === "active").length;
  let slots = MAX_CONCURRENT - active;
  for (const item of items) {
    if (slots <= 0) break;
    if (item.status === "queued") {
      startItem(item);
      slots--;
    }
  }
}
