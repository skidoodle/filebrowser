import { UploadSimpleIcon } from "@phosphor-icons/react";

export function DropUploadOverlay({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <div className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center bg-kumo-canvas/80 backdrop-blur-sm animate-[fadeIn_150ms_ease-out]">
      <div className="border-kumo-info flex flex-col items-center gap-3 rounded-2xl border-2 border-dashed px-12 py-10">
        <UploadSimpleIcon size={40} weight="duotone" className="text-kumo-info" />
        <p className="text-kumo-default text-base font-semibold">Drop files to upload</p>
        <p className="text-kumo-subtle text-sm">Files will be uploaded to this folder</p>
      </div>
    </div>
  );
}
