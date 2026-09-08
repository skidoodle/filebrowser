import { create } from "zustand";

interface SelectionState {
  selected: Set<string>;
  toggle: (path: string) => void;
  selectAll: (paths: string[]) => void;
  clear: () => void;
}

export const useSelection = create<SelectionState>()((set) => ({
  selected: new Set<string>(),
  toggle: (path) =>
    set((state) => {
      const next = new Set(state.selected);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return { selected: next };
    }),
  selectAll: (paths) => set({ selected: new Set(paths) }),
  clear: () => set({ selected: new Set<string>() }),
}));
