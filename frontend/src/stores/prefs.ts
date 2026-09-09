import { useEffect, useState } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { SortBy, SortOrder } from "../api/client";

export type ViewMode = "list" | "mosaic" | "gallery";
export type Theme = "system" | "dark" | "light";

interface PrefsState {
  viewMode: ViewMode;
  sortBy: SortBy;
  sortOrder: SortOrder;
  theme: Theme;
  infoPanel: boolean;
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
    { name: "filebrowser-prefs" },
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
