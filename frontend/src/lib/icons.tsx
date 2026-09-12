import {
  FileIcon,
  FilePdfIcon,
  FileTextIcon,
  FolderIcon,
  ImageSquareIcon,
  MusicNoteIcon,
  VideoCameraIcon,
} from "@phosphor-icons/react";
import type { FileType } from "../types";

export function FileTypeIcon({
  type,
  size = 32,
  className = "",
}: {
  type: FileType;
  size?: number;
  className?: string;
}) {
  const cls = `shrink-0 ${className}`.trim();
  switch (type) {
    case "dir":
      return <FolderIcon size={size} weight="fill" className={`text-blue-500 ${cls}`} />;
    case "image":
      return <ImageSquareIcon size={size} weight="fill" className={`text-purple-400 ${cls}`} />;
    case "video":
      return <VideoCameraIcon size={size} weight="fill" className={`text-amber-400 ${cls}`} />;
    case "audio":
      return <MusicNoteIcon size={size} weight="fill" className={`text-emerald-400 ${cls}`} />;
    case "pdf":
      return <FilePdfIcon size={size} weight="fill" className={`text-red-400 ${cls}`} />;
    case "text":
      return <FileTextIcon size={size} weight="fill" className={`text-sky-400 ${cls}`} />;
    default:
      return <FileIcon size={size} weight="fill" className={`text-kumo-subtle ${cls}`} />;
  }
}
