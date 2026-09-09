import { Button, Dialog, Input } from "@cloudflare/kumo";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { isReservedPath } from "../lib/routes";

interface MoveDialogProps {
  from: string;
  onClose: () => void;
}

export function MoveDialog({ from, onClose }: MoveDialogProps) {
  const name = from.split("/").pop() ?? from;
  const parent = from.includes("/") ? from.slice(0, from.lastIndexOf("/")) : "";
  const [to, setTo] = useState(from);
  const queryClient = useQueryClient();

  const destination = to.trim();
  // Moving an entry onto a route-claimed root name would make it
  // unreachable; the backend rejects it too.
  const reserved = destination !== "" && isReservedPath(destination);

  const move = useMutation({
    mutationFn: () => api.move(from, destination),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["list"] });
      void queryClient.invalidateQueries({ queryKey: ["usage"] });
      onClose();
    },
  });

  const changed = destination !== "" && destination !== from;

  return (
    <Dialog.Root
      open={from !== ""}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <Dialog className="p-6 top-20 sm:top-24 z-50">
        <Dialog.Title>Move or rename</Dialog.Title>
        <Dialog.Description>
          Enter the destination path for <span className="font-semibold">{name}</span>. Same folder renames it; a
          different folder moves it.
        </Dialog.Description>

        <form
          className="mt-4 flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (changed && !reserved && !move.isPending) move.mutate();
          }}
        >
          <Input autoFocus value={to} onChange={(e) => setTo(e.target.value)} placeholder={`${parent ? parent + "/" : ""}new-name${name.includes(".") ? name.slice(name.lastIndexOf(".")) : ".txt"}`} />
          {reserved && <p className="text-kumo-danger text-sm">This name is reserved by the application.</p>}
          {move.error instanceof Error && <p className="text-kumo-danger text-sm">{move.error.message}</p>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" loading={move.isPending} disabled={!changed || reserved}>
              Move
            </Button>
          </div>
        </form>
      </Dialog>
    </Dialog.Root>
  );
}
