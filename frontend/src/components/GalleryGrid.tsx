import { Empty } from "@cloudflare/kumo";
import type { MouseEvent as ReactMouseEvent } from "react";
import { api } from "../api/client";
import type { FileInfo } from "../types";

export interface GalleryGridProps {
  items: FileInfo[];
  selected: Set<string>;
  onSelect: (file: FileInfo, additive: boolean) => void;
  onOpen: (file: FileInfo) => void;
  onMenu: (file: FileInfo, e: ReactMouseEvent<HTMLElement>) => void;
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
        <button
          key={file.path}
          type="button"
          data-file-item
          data-path={file.path}
          draggable={false}
          onDragStart={(e) => e.preventDefault()}
          onClick={(e) => onSelect(file, e.ctrlKey || e.metaKey || e.shiftKey)}
          onDoubleClick={() => onOpen(file)}
          onContextMenu={(e) => onMenu(file, e)}
          className={`bg-kumo-base ring-kumo-hairline group relative overflow-hidden rounded-xl ring-1 ${
            selected.has(file.path) ? "ring-2 ring-kumo-info" : ""
          }`}
        >
          <img
            src={api.thumbUrl(file.path)}
            alt={file.name}
            loading="lazy"
            draggable={false}
            className="aspect-square w-full object-cover pointer-events-none select-none"
          />
          <p className="truncate px-3 py-2 text-left text-xs leading-5 select-none" title={file.name}>
            {file.name}
          </p>
        </button>
      ))}
    </div>
  );
}
