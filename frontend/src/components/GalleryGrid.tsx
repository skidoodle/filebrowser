import { Empty } from "@cloudflare/kumo";
import { DotsThreeVerticalIcon } from "@phosphor-icons/react";
import type { MouseEvent as ReactMouseEvent } from "react";
import { api } from "../api/client";
import { useLongPress } from "../lib/useLongPress";
import type { FileInfo } from "../types";

export interface GalleryGridProps {
  items: FileInfo[];
  selected: Set<string>;
  onSelect: (file: FileInfo, additive: boolean) => void;
  onOpen: (file: FileInfo) => void;
  onMenu: (file: FileInfo, e: ReactMouseEvent<HTMLElement>) => void;
}

function GalleryItem({
  file,
  isSelected,
  onSelect,
  onOpen,
  onMenu,
}: {
  file: FileInfo;
  isSelected: boolean;
  onSelect: (file: FileInfo, additive: boolean) => void;
  onOpen: (file: FileInfo) => void;
  onMenu: (file: FileInfo, e: ReactMouseEvent<HTMLElement>) => void;
}) {
  const longPress = useLongPress({
    onLongPress: (coords) => {
      onMenu(file, coords as unknown as ReactMouseEvent<HTMLElement>);
    },
  });

  return (
    <div
      role="button"
      tabIndex={0}
      data-file-item
      data-path={file.path}
      {...longPress.handlers}
      onClick={(e) => onSelect(file, e.ctrlKey || e.metaKey || e.shiftKey)}
      onDoubleClick={() => onOpen(file)}
      onKeyDown={(e) => {
        if (e.key === "Enter") onOpen(file);
      }}
      onContextMenu={(e) => {
        e.preventDefault();
        onMenu(file, e);
      }}
      className={`bg-kumo-base ring-kumo-hairline group relative overflow-hidden rounded-xl ring-1 cursor-pointer ${
        isSelected ? "ring-2 ring-kumo-info" : ""
      }`}
    >
      <img
        src={api.thumbUrl(file.path)}
        alt={file.name}
        loading="lazy"
        draggable={false}
        className="aspect-square w-full object-cover pointer-events-none select-none"
      />
      <div className="flex items-center justify-between gap-1 px-3 py-2">
        <p className="truncate text-left text-xs leading-5 select-none font-medium flex-1" title={file.name}>
          {file.name}
        </p>
        <button
          type="button"
          aria-label={`Actions for ${file.name}`}
          title="Actions"
          onClick={(e) => {
            e.stopPropagation();
            const rect = e.currentTarget.getBoundingClientRect();
            onMenu(file, {
              clientX: rect.left,
              clientY: rect.bottom + 4,
              preventDefault: () => {},
              stopPropagation: () => {},
            } as unknown as ReactMouseEvent<HTMLElement>);
          }}
          className="text-kumo-subtle hover:text-kumo-default hover:bg-kumo-tint -mr-1.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg cursor-pointer md:hidden"
        >
          <DotsThreeVerticalIcon size={18} weight="bold" />
        </button>
      </div>
    </div>
  );
}

export function GalleryGrid({ items, selected, onSelect, onOpen, onMenu }: GalleryGridProps) {
  if (items.length === 0) {
    return (
      <Empty
        title="No images in this folder"
        description="Gallery view shows image files; try mosaic or list for everything else."
      />
    );
  }
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-3">
      {items.map((file) => (
        <GalleryItem
          key={file.path}
          file={file}
          isSelected={selected.has(file.path)}
          onSelect={onSelect}
          onOpen={onOpen}
          onMenu={onMenu}
        />
      ))}
    </div>
  );
}
