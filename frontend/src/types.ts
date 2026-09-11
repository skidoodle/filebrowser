import { z } from "zod";

export const FileTypeSchema = z.enum(["dir", "video", "audio", "image", "pdf", "text", "blob"]);
export type FileType = z.infer<typeof FileTypeSchema>;

export const FileInfoSchema = z.object({
  name: z.string(),
  path: z.string(),
  size: z.number(),
  modified: z.string(),
  isDir: z.boolean(),
  type: FileTypeSchema,
  mime: z.string().optional(),
  extension: z.string().optional(),
  private: z.boolean().optional(),
});
export type FileInfo = z.infer<typeof FileInfoSchema>;

export const ListingSchema = z.object({
  items: z.array(FileInfoSchema),
  numDirs: z.number(),
  numFiles: z.number(),
  total: z.number(),
});
export type Listing = z.infer<typeof ListingSchema>;

export const UsageSchema = z.object({
  used: z.number(),
  total: z.number(),
});
export type Usage = z.infer<typeof UsageSchema>;

export const HealthSchema = z.object({
  status: z.string(),
  version: z.string(),
  commit: z.string(),
});
export type Health = z.infer<typeof HealthSchema>;

export const FileMetaSchema = FileInfoSchema.extend({
  checksums: z.record(z.string(), z.string()).optional(),
});
export type FileMeta = z.infer<typeof FileMetaSchema>;

export const MeSchema = z.object({
  admin: z.boolean(),
  insecure: z.boolean(),
  initialized: z.boolean(),
});
export type Me = z.infer<typeof MeSchema>;
