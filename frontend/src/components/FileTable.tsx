import { DotsThreeVerticalIcon } from "@phosphor-icons/react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef, type MouseEvent as ReactMouseEvent, type RefObject } from "react";
import { useFolderDrop, useItemDrag } from "./dnd";
import { useMe } from "../lib/useMe";
import { canWriteIn, canWritePath } from "../lib/permissions";
import { formatBytes, formatRelative } from "../lib/format";
import { FileTypeIcon } from "../lib/icons";
import { mergeRefs } from "../lib/refs";
import { isTouchPointer } from "../lib/pointer";
import type { FileInfo } from "../types";

export interface FileTableProps {
  items: FileInfo[];
  selected: Set<string>;
  onSelect: (file: FileInfo, additive: boolean) => void;
  onOpen: (file: FileInfo) => void;
  onMenu: (file: FileInfo, e: ReactMouseEvent<HTMLElement>) => void;
  scrollRef: RefObject<HTMLDivElement | null>;
}

export function FileTable({
  items,
  selected,
  onSelect,
  onOpen,
  onMenu,
  scrollRef,
}: FileTableProps) {
  const tableRef = useRef<HTMLTableElement>(null);

  // eslint-disable-next-line react-hooks/incompatible-library
  const rowVirtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 49,
    overscan: 10,
  });

  const virtualItems = rowVirtualizer.getVirtualItems();
  const paddingTop = virtualItems.length > 0 ? virtualItems[0].start : 0;
  const paddingBottom =
    virtualItems.length > 0
      ? rowVirtualizer.getTotalSize() - virtualItems[virtualItems.length - 1].end
      : 0;

  return (
    <table ref={tableRef} className="w-full table-fixed border-collapse text-sm">
      <thead>
        <tr className="text-kumo-subtle border-kumo-hairline border-b text-left">
          <th className="px-2 py-2.5 font-medium sm:px-3">Name</th>
          <th className="w-20 px-2 py-2.5 font-medium sm:w-28 sm:px-3">Size</th>
          <th className="hidden w-40 px-3 py-2.5 font-medium sm:table-cell">Modified</th>
          <th className="w-10 px-1 py-2.5 sm:w-12 md:hidden"><span className="sr-only">Actions</span></th>
        </tr>
      </thead>
      <tbody>
        {paddingTop > 0 && (
          <tr aria-hidden="true">
            <td colSpan={4} style={{ height: `${paddingTop}px` }} />
          </tr>
        )}
        {virtualItems.map((virtualRow) => {
          const file = items[virtualRow.index];
          if (!file) return null;
          return (
            <FileRow
              key={file.path}
              file={file}
              isSelected={selected.has(file.path)}
              onSelect={onSelect}
              onOpen={onOpen}
              onMenu={onMenu}
            />
          );
        })}
        {paddingBottom > 0 && (
          <tr aria-hidden="true">
            <td colSpan={4} style={{ height: `${paddingBottom}px` }} />
          </tr>
        )}
      </tbody>
    </table>
  );
}

interface FileRowProps {
  file: FileInfo;
  isSelected: boolean;
  onSelect: (file: FileInfo, additive: boolean) => void;
  onOpen: (file: FileInfo) => void;
  onMenu: (file: FileInfo, e: ReactMouseEvent<HTMLElement>) => void;
}

function FileRow({ file, isSelected, onSelect, onOpen, onMenu }: FileRowProps) {
  const me = useMe().data;
  const canWrite = canWritePath(me, file.path);
  const drag = useItemDrag(file, canWrite);
  const drop = useFolderDrop({ path: file.path, isDir: file.isDir }, canWriteIn(me, file.path));

  return (
    <tr
      ref={mergeRefs(drag.ref, drop.ref)}
      {...drag.handleProps}
      data-file-item
      data-path={file.path}
      onClick={(e) => onSelect(file, e.ctrlKey || e.metaKey || e.shiftKey)}
      onDoubleClick={() => onOpen(file)}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
        if (isTouchPointer(e)) return;
        onMenu(file, e);
      }}
      className={`border-kumo-hairline hover:bg-kumo-tint cursor-pointer border-b select-none touch-manipulation ${isSelected ? "bg-kumo-info-tint/50" : ""
        } ${drag.isDragging ? "opacity-30 outline-2 outline-dashed outline-kumo-info" : ""} ${drop.isOver ? "bg-kumo-success-tint/70 shadow-inner" : ""
        }`}
    >
      <td className="px-2 py-3 sm:px-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <FileTypeIcon type={file.type} size={22} className="shrink-0" />
          <span className="truncate leading-6 font-medium min-w-0 flex-1" title={file.name}>
            {file.name}
          </span>
        </div>
      </td>
      <td className="text-kumo-subtle truncate px-2 py-3 leading-6 sm:px-3">
        {file.isDir ? "—" : formatBytes(file.size)}
      </td>
      <td className="text-kumo-subtle hidden truncate px-3 py-3 leading-6 sm:table-cell">
        {formatRelative(file.modified)}
      </td>
      <td className="w-10 px-1 py-3 text-right sm:w-12 md:hidden" onClick={(e) => e.stopPropagation()}>
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
              preventDefault: () => { },
              stopPropagation: () => { },
            } as unknown as ReactMouseEvent<HTMLElement>);
          }}
          className="text-kumo-subtle hover:text-kumo-default hover:bg-kumo-tint inline-flex h-8 w-8 items-center justify-center rounded-lg cursor-pointer"
        >
          <DotsThreeVerticalIcon size={18} weight="bold" />
        </button>
      </td>
    </tr>
  );
}
