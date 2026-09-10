import { CommandPalette } from "@cloudflare/kumo";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import { FileTypeIcon } from "../lib/icons";
import { parentPath } from "../lib/path";
import type { FileInfo } from "../types";

interface SearchOverlayProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (file: FileInfo) => void;
  currentDir?: string;
}

interface ResultGroup {
  label: string;
  items: FileInfo[];
}

export function SearchOverlay({ open, onOpenChange, onSelect, currentDir }: SearchOverlayProps) {
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [scopedOverride, setScopedOverride] = useState<boolean | null>(null);

  const hasDir = Boolean(currentDir && currentDir !== ".");
  const scoped = scopedOverride !== null ? scopedOverride : hasDir;
  const activePath = scoped && hasDir ? currentDir : undefined;

  useEffect(() => {
    const t = setTimeout(() => setDebounced(query), 150);
    return () => clearTimeout(t);
  }, [query]);

  const search = useQuery({
    queryKey: ["search", debounced, activePath],
    queryFn: () => api.search(debounced, 50, activePath),
    enabled: open && debounced.trim().length > 0,
    placeholderData: (prev) => prev,
  });

  const results = search.data ?? [];
  const groupLabel = activePath ? `Results in /${activePath}` : "Results";
  const groups: ResultGroup[] =
    results.length > 0 ? [{ label: groupLabel, items: results }] : [];

  return (
    <CommandPalette.Root
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) {
          setQuery("");
          setScopedOverride(null);
        }
      }}
      items={groups}
      value={query}
      onValueChange={setQuery}
      filter={() => true}
      getSelectableItems={(gs) => gs.flatMap((g) => g.items)}
      onSelect={(item) => {
        onSelect(item);
        onOpenChange(false);
        setQuery("");
      }}
    >
      <CommandPalette.Input
        autoFocus
        placeholder={activePath ? `Search in /${activePath}…` : "Search files and folders…"}
      />
      <CommandPalette.List>
        <CommandPalette.Results>
          {(group) => (
            <CommandPalette.Group key={group.label} items={group.items}>
              <CommandPalette.GroupLabel>{group.label}</CommandPalette.GroupLabel>
              <CommandPalette.Items>
                {(item: FileInfo) => (
                  <CommandPalette.ResultItem
                    key={item.path}
                    title={item.name}
                    breadcrumbs={[parentPath(item.path)]}
                    icon={<FileTypeIcon type={item.type} size={18} />}
                    value={item}
                    onClick={() => {
                      onSelect(item);
                      onOpenChange(false);
                      setQuery("");
                    }}
                  />
                )}
              </CommandPalette.Items>
            </CommandPalette.Group>
          )}
        </CommandPalette.Results>
        {search.isFetching && <CommandPalette.Loading />}
        {!search.isFetching && results.length === 0 && debounced.trim().length > 0 && (
          <CommandPalette.Empty>No results found</CommandPalette.Empty>
        )}
      </CommandPalette.List>
      <CommandPalette.Footer>
        {currentDir && currentDir !== "." && (
          <button
            type="button"
            onClick={() => setScopedOverride((s) => (s !== null ? !s : !hasDir))}
            className="text-kumo-info hover:underline cursor-pointer mr-auto text-xs font-medium"
          >
            {scoped
              ? `Scope: /${currentDir} (click for all)`
              : "Scope: All files (click for folder)"}
          </button>
        )}
        <span>↑↓ navigate</span>
        <span>↵ open</span>
        <span>esc close</span>
      </CommandPalette.Footer>
    </CommandPalette.Root>
  );
}
