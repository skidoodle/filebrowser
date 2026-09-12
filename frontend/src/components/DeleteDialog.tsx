import { Button, Dialog } from "@cloudflare/kumo";

export interface DeleteDialogProps {
  paths: string[] | null;
  loading: boolean;
  onClose: () => void;
  onConfirm: () => void;
}

export function DeleteDialog({ paths, loading, onClose, onConfirm }: DeleteDialogProps) {
  return (
    <Dialog.Root
      open={paths !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <Dialog className="p-6 top-20 sm:top-24 z-50">
        <Dialog.Title>Delete</Dialog.Title>
        <Dialog.Description>
          {paths !== null && paths.length > 1
            ? `Permanently delete ${paths.length} items? This cannot be undone.`
            : "Permanently delete this item? This cannot be undone."}
        </Dialog.Description>
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            loading={loading}
            onClick={onConfirm}
          >
            Delete
          </Button>
        </div>
      </Dialog>
    </Dialog.Root>
  );
}
