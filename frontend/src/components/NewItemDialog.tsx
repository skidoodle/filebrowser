import { Button, Dialog, Input } from "@cloudflare/kumo";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { isReservedPath } from "../lib/routes";
import { joinPath } from "../lib/path";
import { setEditToken } from "../lib/tokens";

export type NewItemKind = "dir" | "file" | null;

interface NewItemDialogProps {
  kind: NewItemKind;
  dir: string;
  onClose: () => void;
  onCreate: (path: string, kind: "dir" | "file") => void;
}

export function NewItemDialog({ kind, dir, onClose, onCreate }: NewItemDialogProps) {
  const [name, setName] = useState("");
  const queryClient = useQueryClient();

  const trimmed = name.trim();
  // Root-level entries named after an application route would be
  // unreachable in the browser; the backend rejects them too.
  const reserved = kind !== null && trimmed !== "" && isReservedPath(joinPath(dir, trimmed));

  const create = useMutation({
    mutationFn: async () => {
      const path = joinPath(dir, trimmed);
      if (kind === "dir") {
        await api.createDir(path);
      } else if (kind === "file") {
        const res = await api.createFile(path);
        if (res.edit_token) {
          setEditToken(path, res.edit_token);
        }
      }
      return path;
    },
    onSuccess: (path) => {
      const createdKind = kind ?? "file";
      void queryClient.invalidateQueries({ queryKey: ["list"] });
      void queryClient.invalidateQueries({ queryKey: ["usage"] });
      setName("");
      onCreate(path, createdKind);
    },
  });

  const title = kind === "dir" ? "New folder" : "New file";
  const pending = create.isPending;
  const error = reserved
    ? "This name is reserved by the application."
    : create.error instanceof Error
      ? create.error.message
      : null;

  return (
    <Dialog.Root
      open={kind !== null}
      onOpenChange={(open) => {
        if (!open) {
          setName("");
          onClose();
        }
      }}
    >
      <Dialog className="p-6 top-20 sm:top-24 z-50">
        <Dialog.Title>{title}</Dialog.Title>
        <Dialog.Description>Name entries exactly as they should appear, including extensions for files.</Dialog.Description>

        <form
          className="mt-4 flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (trimmed && !reserved) create.mutate();
          }}
        >
          <Input
            autoFocus
            value={name}
            placeholder={kind === "dir" ? "documents" : "notes.txt"}
            onChange={(e) => setName(e.target.value)}
          />
          {error && <p className="text-kumo-danger text-sm">{error}</p>}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setName("");
                onClose();
              }}
            >
              Cancel
            </Button>
            <Button type="submit" variant="primary" loading={pending} disabled={!trimmed || reserved}>
              Create
            </Button>
          </div>
        </form>
      </Dialog>
    </Dialog.Root>
  );
}
