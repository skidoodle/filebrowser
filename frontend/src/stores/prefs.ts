import { useEffect, useState } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";
import { z } from "zod";
import type { SortBy, SortOrder } from "../api/client";

export const ViewModeSchema = z.enum(["list", "mosaic", "gallery"]);
export type ViewMode = z.infer<typeof ViewModeSchema>;

export const ThemeSchema = z.enum(["system", "dark", "light"]);
export type Theme = z.infer<typeof ThemeSchema>;

const SortBySchema = z.enum(["name", "size", "modified"]);
const SortOrderSchema = z.enum(["asc", "desc"]);

export const PrefsDataSchema = z.object({
  viewMode: ViewModeSchema.catch("mosaic"),
  sortBy: SortBySchema.catch("name"),
  sortOrder: SortOrderSchema.catch("asc"),
  theme: ThemeSchema.catch("system"),
  infoPanel: z.boolean().catch(false),
});
export type PrefsData = z.infer<typeof PrefsDataSchema>;

interface PrefsState extends PrefsData {
  setViewMode: (v: ViewMode) => void;
  setSortBy: (v: SortBy) => void;
  setSortOrder: (v: SortOrder) => void;
  setTheme: (t: Theme) => void;
  toggleInfoPanel: () => void;
}

export const usePrefs = create<PrefsState>()(
  persist(
    (set) => ({
      viewMode: "mosaic",
      sortBy: "name",
      sortOrder: "asc",
      theme: "system",
      infoPanel: false,
      setViewMode: (viewMode) => set({ viewMode }),
      setSortBy: (sortBy) => set({ sortBy }),
      setSortOrder: (sortOrder) => set({ sortOrder }),
      setTheme: (theme) => set({ theme }),
      toggleInfoPanel: () => set((s) => ({ infoPanel: !s.infoPanel })),
    }),
    {
      name: "filebrowser-prefs",
      merge: (persistedState, currentState) => {
        const parsed = PrefsDataSchema.safeParse(persistedState);
        return {
          ...currentState,
          ...(parsed.success ? parsed.data : {}),
        };
      },
    },
  ),
);

export function useEffectiveTheme(): "dark" | "light" {
  const theme = usePrefs((s) => s.theme);
  const [systemDark, setSystemDark] = useState(
    () => typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches,
  );

  useEffect(() => {
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => setSystemDark(mql.matches);
    mql.addEventListener("change", apply);
    return () => mql.removeEventListener("change", apply);
  }, []);

  if (theme === "dark") return "dark";
  if (theme === "light") return "light";
  return systemDark ? "dark" : "light";
}
