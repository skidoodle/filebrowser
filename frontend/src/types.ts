export type FileType = "dir" | "video" | "audio" | "image" | "pdf" | "text" | "blob";

export interface FileInfo {
  name: string;
  path: string;
  size: number;
  modified: string;
  isDir: boolean;
  type: FileType;
  mime?: string;
  extension?: string;
  private?: boolean;
}

export interface Listing {
  items: FileInfo[];
  numDirs: number;
  numFiles: number;
  total: number;
}

export interface Usage {
  used: number;
  total: number;
}

export interface Health {
  status: string;
  version: string;
  commit: string;
}

export interface FileMeta extends FileInfo {
  checksums?: Record<string, string>;
}

export interface Me {
  admin: boolean;
  insecure: boolean;
  initialized: boolean;
}
