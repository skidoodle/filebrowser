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

export const AccessPolicySchema = z.enum(["public", "readonly", "private"]);
export type AccessPolicy = z.infer<typeof AccessPolicySchema>;

export const MeSchema = z.object({
  admin: z.boolean(),
  insecure: z.boolean(),
  initialized: z.boolean(),
  username: z.string().optional(),
  scope: z.string().optional(),
  access_policy: AccessPolicySchema.optional(),
});
export type Me = z.infer<typeof MeSchema>;

export const UserSchema = z.object({
  id: z.number(),
  username: z.string(),
  admin: z.boolean(),
  scope: z.string(),
  isOriginal: z.boolean(),
});
export type User = z.infer<typeof UserSchema>;

export const UsernameSchema = z
  .string()
  .trim()
  .min(1, "Username is required")
  .max(64, "Username must be 64 characters or fewer")
  .regex(/^[a-zA-Z0-9._-]+$/, "Username may only contain letters, digits, and . _ -");

export const PasswordSchema = z
  .string()
  .min(8, "Password must be at least 8 characters");

export const NewUserSchema = z.object({
  username: UsernameSchema,
  password: PasswordSchema,
  admin: z.boolean(),
  scope: z.string(),
});
export type NewUser = z.infer<typeof NewUserSchema>;
