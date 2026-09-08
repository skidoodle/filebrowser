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
}

interface ResultGroup {
  label: string;
  items: FileInfo[];
}

export function SearchOverlay({ open, onOpenChange, onSelect }: SearchOverlayProps) {
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");

  useEffect(() => {
    const t = setTimeout(() => setDebounced(query), 250);
    return () => clearTimeout(t);
  }, [query]);

  const search = useQuery({
    queryKey: ["search", debounced],
    queryFn: () => api.search(debounced, 50),
    enabled: open && debounced.trim().length > 0,
    placeholderData: (prev) => prev,
  });

  const results = search.data ?? [];
  const groups: ResultGroup[] =
    results.length > 0 ? [{ label: "Results", items: results }] : [];

  return (
    <CommandPalette.Root
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) setQuery("");
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
      <CommandPalette.Input autoFocus placeholder="Search files and folders…" />
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
        <span>↑↓ navigate</span>
        <span>↵ open</span>
        <span>esc close</span>
      </CommandPalette.Footer>
    </CommandPalette.Root>
  );
}
