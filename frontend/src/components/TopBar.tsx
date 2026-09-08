import {
  ArrowsDownUpIcon,
  ArrowDownIcon,
  ArrowUpIcon,
  CheckSquareIcon,
  DesktopIcon,
  DownloadSimpleIcon,
  InfoIcon,
  ListBulletsIcon,
  MagnifyingGlassIcon,
  MoonIcon,
  SquareIcon,
  SquaresFourIcon,
  SunIcon,
  UploadSimpleIcon,
} from "@phosphor-icons/react";
import { Button, DropdownMenu } from "@cloudflare/kumo";
import { useRef } from "react";
import type { SortBy } from "../api/client";
import { usePrefs, type Theme, type ViewMode } from "../stores/prefs";
import { MenuItemRow } from "./MenuItemRow";
const VIEW_LABELS: Record<ViewMode, string> = {
  list: "List",
  mosaic: "Mosaic",
  gallery: "Gallery",
};

const SORT_LABELS: Record<SortBy, string> = {
  name: "Name",
  size: "Size",
  modified: "Modified",
};

const THEME_LABELS: Record<Theme, string> = {
  system: "System",
  dark: "Dark",
  light: "Light",
};

function ThemeIcon({ theme, size }: { theme: Theme; size: number }) {
  if (theme === "dark") return <MoonIcon size={size} />;
  if (theme === "light") return <SunIcon size={size} />;
  return <DesktopIcon size={size} />;
}

interface TopBarProps {
  onSearch: () => void;
  onUploadFiles?: (files: File[]) => void;
  onDownload: () => void;
  canDownload: boolean;
  onSelectAll: () => void;
  allSelected: boolean;
}

export function TopBar({
  onSearch,
  onUploadFiles,
  onDownload,
  canDownload,
  onSelectAll,
  allSelected,
}: TopBarProps) {
  const prefs = usePrefs();
  const fileInput = useRef<HTMLInputElement>(null);

  return (
    <div className="border-kumo-hairline bg-kumo-canvas/90 sticky top-0 z-30 flex h-14 items-center gap-2 border-b px-3 md:px-4">
      <button
        type="button"
        onClick={onSearch}
        className="ring-kumo-hairline text-kumo-subtle hover:text-kumo-default hidden h-9 w-72 items-center gap-2.5 rounded-lg px-3 text-sm ring-1 md:flex"
      >
        <MagnifyingGlassIcon size={18} />
        Search…
        <kbd className="bg-kumo-tint text-kumo-subtle ml-auto rounded px-1.5 py-0.5 text-xs">
          Ctrl K
        </kbd>
      </button>
      <Button
        variant="ghost"
        shape="square"
        aria-label="Search"
        icon={<MagnifyingGlassIcon size={20} />}
        onClick={onSearch}
        className="md:hidden"
      />

      <div className="ml-auto flex items-center gap-1">
        <button
          type="button"
          aria-label={allSelected ? "Clear selection" : "Select all entries"}
          title={allSelected ? "Clear selection" : "Select all"}
          onClick={onSelectAll}
          className={`hover:bg-kumo-tint mx-1.5 rounded-md p-1.5 ${allSelected ? "text-kumo-info" : "text-kumo-subtle hover:text-kumo-default"
            }`}
        >
          {allSelected ? <CheckSquareIcon size={20} weight="fill" /> : <SquareIcon size={20} />}
        </button>

        <div className="bg-kumo-hairline mx-1 h-6 w-px" />

        <DropdownMenu>
          <DropdownMenu.Trigger
            render={(p) => (
              <Button {...p} variant="ghost" shape="square" aria-label="Change view" icon={<SquaresFourIcon size={20} weight="fill" />} />
            )}
          />
          <DropdownMenu.Content align="end" className="min-w-44 z-60">
            {(Object.keys(VIEW_LABELS) as ViewMode[]).map((mode) => (
              <MenuItemRow
                key={mode}
                icon={mode === "list" ? <ListBulletsIcon size={16} /> : <SquaresFourIcon size={16} />}
                label={VIEW_LABELS[mode]}
                active={prefs.viewMode === mode}
                onClick={() => prefs.setViewMode(mode)}
              />
            ))}
          </DropdownMenu.Content>
        </DropdownMenu>

        <DropdownMenu>
          <DropdownMenu.Trigger
            render={(p) => (
              <Button {...p} variant="ghost" shape="square" aria-label="Change sorting" icon={<ArrowsDownUpIcon size={20} weight="fill" />} />
            )}
          />
          <DropdownMenu.Content align="end" className="min-w-44 z-60">
            {(Object.keys(SORT_LABELS) as SortBy[]).map((key) => (
              <MenuItemRow
                key={key}
                label={SORT_LABELS[key]}
                active={prefs.sortBy === key}
                onClick={() => prefs.setSortBy(key)}
              />
            ))}
            <DropdownMenu.Separator />
            <MenuItemRow
              icon={<ArrowDownIcon size={16} />}
              label="Ascending"
              active={prefs.sortOrder === "asc"}
              onClick={() => prefs.setSortOrder("asc")}
            />
            <MenuItemRow
              icon={<ArrowUpIcon size={16} />}
              label="Descending"
              active={prefs.sortOrder === "desc"}
              onClick={() => prefs.setSortOrder("desc")}
            />
          </DropdownMenu.Content>
        </DropdownMenu>

        <Button
          variant="ghost"
          shape="square"
          aria-label="Download selected"
          title={canDownload ? "Download selection" : "Select entries to download"}
          icon={<DownloadSimpleIcon size={20} weight="fill" />}
          disabled={!canDownload}
          onClick={onDownload}
        />

        {onUploadFiles && (
          <>
            <Button
              variant="ghost"
              shape="square"
              aria-label="Upload files"
              icon={<UploadSimpleIcon size={20} weight="fill" />}
              onClick={() => fileInput.current?.click()}
            />
            <input
              ref={fileInput}
              type="file"
              multiple
              hidden
              onChange={(e) => {
                if (e.target.files) onUploadFiles(Array.from(e.target.files));
                e.target.value = "";
              }}
            />
          </>
        )}

        <Button
          variant={prefs.infoPanel ? "secondary" : "ghost"}
          shape="square"
          aria-label="Toggle details panel"
          icon={<InfoIcon size={20} weight="fill" />}
          onClick={prefs.toggleInfoPanel}
          className="max-md:hidden"
        />

        <div className="bg-kumo-hairline mx-1 h-6 w-px" />

        <DropdownMenu>
          <DropdownMenu.Trigger
            render={(p) => (
              <Button
                {...p}
                variant="ghost"
                shape="square"
                aria-label="Change theme"
                icon={<ThemeIcon theme={prefs.theme} size={20} />}
              />
            )}
          />
          <DropdownMenu.Content align="end" className="min-w-44 z-60">
            {(Object.keys(THEME_LABELS) as Theme[]).map((t) => (
              <MenuItemRow
                key={t}
                icon={<ThemeIcon theme={t} size={16} />}
                label={THEME_LABELS[t]}
                active={prefs.theme === t}
                onClick={() => prefs.setTheme(t)}
              />
            ))}
          </DropdownMenu.Content>
        </DropdownMenu>
      </div>
    </div>
  );
}
