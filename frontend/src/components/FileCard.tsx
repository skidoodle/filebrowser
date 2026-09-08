import { CheckIcon } from "@phosphor-icons/react";
import type { MouseEvent as ReactMouseEvent } from "react";
import { useFolderDrop, useItemDrag } from "./dnd";
import { useMe } from "../lib/useMe";
import { canWriteIn, canWritePath } from "../lib/permissions";
import { formatBytes, formatRelative } from "../lib/format";
import { FileTypeIcon } from "../lib/icons";
import { mergeRefs } from "../lib/refs";
import type { FileInfo } from "../types";

interface FileCardProps {
  file: FileInfo;
  selected: boolean;
  onSelect: (file: FileInfo, additive: boolean) => void;
  onOpen: (file: FileInfo) => void;
  onMenu: (file: FileInfo, e: ReactMouseEvent<HTMLElement>) => void;
}

export function FileCard({ file, selected, onSelect, onOpen, onMenu }: FileCardProps) {
  const me = useMe().data;
  const canWrite = canWritePath(me, file.path);
  const drag = useItemDrag(file, canWrite);
  const drop = useFolderDrop({ path: file.path, isDir: file.isDir }, canWriteIn(me, file.path));

  return (
    <div
      role="button"
      tabIndex={0}
      ref={mergeRefs(drag.ref, drop.ref)}
      {...drag.handleProps}
      data-file-item
      data-path={file.path}
      onClick={(e) => onSelect(file, e.ctrlKey || e.metaKey || e.shiftKey)}
      onDoubleClick={() => onOpen(file)}
      onKeyDown={(e) => {
        if (e.key === "Enter") onOpen(file);
      }}
      onContextMenu={(e) => {
        e.preventDefault();
        onMenu(file, e);
      }}
      className={`bg-kumo-base ring-kumo-hairline relative flex cursor-grab active:cursor-grabbing items-start gap-3 rounded-xl p-4 ring-1 transition-shadow hover:shadow-md ${selected ? "ring-2 ring-kumo-info" : ""
        } ${drag.isDragging ? "opacity-30 outline-2 outline-dashed outline-kumo-info" : ""} ${drop.isOver ? "scale-[1.03] ring-2 ring-kumo-success shadow-lg" : ""
        }`}
    >
      <FileTypeIcon type={file.type} size={40} />
      <div className="min-w-0 flex-1">
        <p className="truncate leading-6 text-sm font-semibold" title={file.name}>
          {file.name}
        </p>
        <p className="text-kumo-subtle mt-0.5 text-sm">
          {file.isDir ? "—" : formatBytes(file.size)}
        </p>
        <p className="text-kumo-subtle mt-0.5 text-sm">{formatRelative(file.modified)}</p>
      </div>
      {selected && (
        <span className="bg-kumo-info ring-kumo-base absolute -top-1.5 -right-1.5 flex h-5 w-5 items-center justify-center rounded-full ring-2">
          <CheckIcon weight="bold" size={12} className="text-white" />
        </span>
      )}
    </div>
  );
}
