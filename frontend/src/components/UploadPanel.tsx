import {
  ArrowClockwiseIcon,
  CaretDownIcon,
  CaretUpIcon,
  CheckCircleIcon,
  FileIcon,
  WarningCircleIcon,
  XIcon,
} from "@phosphor-icons/react";
import { Button, Meter } from "@cloudflare/kumo";
import { useEffect, useState } from "react";
import { formatBytes } from "../lib/format";
import { useUploads, type UploadItem } from "../stores/uploads";

const DISMISS_DELAY_MS = 3000;

function statusColor(status: UploadItem["status"]): string {
  switch (status) {
    case "done":
      return "text-kumo-success";
    case "error":
      return "text-kumo-danger";
    default:
      return "text-kumo-default";
  }
}

function StatusIcon({ status }: { status: UploadItem["status"] }) {
  if (status === "done") return <CheckCircleIcon size={18} weight="fill" className="shrink-0 text-kumo-success" />;
  if (status === "error") return <WarningCircleIcon size={18} weight="fill" className="shrink-0 text-kumo-danger" />;
  return <FileIcon size={18} className={`shrink-0 ${statusColor(status)}`} />;
}

function UploadRow({ item }: { item: UploadItem }) {
  const retry = useUploads((s) => s.retry);
  const remove = useUploads((s) => s.remove);
  const percent = item.size > 0 ? Math.round((item.uploaded / item.size) * 100) : item.status === "done" ? 100 : 0;
  const speed = item.status === "active" && item.speed && item.speed > 0 ? ` · ${formatBytes(item.speed)}/s` : "";

  return (
    <div className="flex items-center gap-2 py-1.5">
      <StatusIcon status={item.status} />
      <div className="min-w-0 flex-1">
        <p className="truncate leading-6 text-sm" title={item.path}>
          {item.name}
        </p>
        {item.status === "error" ? (
          <p className="text-kumo-danger truncate leading-5 text-xs" title={item.error}>
            {item.error}
          </p>
        ) : (
          <Meter
            label={`${formatBytes(item.uploaded)} / ${formatBytes(item.size)}${speed}`}
            value={percent}
            showValue
            className="mt-1"
          />
        )}
      </div>
      {item.status === "error" && (
        <Button variant="ghost" shape="square" size="sm" aria-label="Retry upload" icon={<ArrowClockwiseIcon />} onClick={() => retry(item.id)} />
      )}
      <Button
        variant="ghost"
        shape="square"
        size="sm"
        aria-label={item.status === "active" ? "Cancel upload" : "Dismiss"}
        title={item.status === "active" ? "Cancel upload" : "Dismiss"}
        icon={<XIcon />}
        onClick={() => remove(item.id)}
      />
    </div>
  );
}

export function UploadPanel() {
  const items = useUploads((s) => s.items);
  const clearFinished = useUploads((s) => s.clearFinished);
  const [open, setOpen] = useState(true);

  const active = items.filter((it) => it.status === "active" || it.status === "queued").length;
  const allDone = items.length > 0 && items.every((it) => it.status === "done");
  const failed = items.filter((it) => it.status === "error").length;

  useEffect(() => {
    if (!allDone) return;
    const timer = setTimeout(clearFinished, DISMISS_DELAY_MS);
    return () => clearTimeout(timer);
  }, [allDone, clearFinished]);

  if (items.length === 0) return null;

  const totalUploaded = items.reduce((acc, it) => acc + it.uploaded, 0);
  const totalSize = items.reduce((acc, it) => acc + it.size, 0);
  const percent = totalSize > 0 ? Math.round((totalUploaded / totalSize) * 100) : 0;

  const title =
    active > 0
      ? `Uploading ${active} of ${items.length}`
      : failed > 0 && !allDone
        ? `Uploaded with ${failed} failed`
        : `Upload${items.length === 1 ? "" : "s"} complete`;

  return (
    <div className="bg-kumo-elevated ring-kumo-line fixed inset-x-3 bottom-3 z-50 rounded-xl p-3 shadow-lg ring sm:inset-x-auto sm:right-4 sm:bottom-4 sm:w-96">
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          shape="square"
          size="sm"
          aria-label={open ? "Collapse uploads" : "Expand uploads"}
          icon={open ? <CaretDownIcon /> : <CaretUpIcon />}
          onClick={() => setOpen(!open)}
        />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{title}</p>
          {items.length > 1 && <Meter label="" value={percent} showValue={false} className="mt-1" />}
        </div>
      </div>
      {open && (
        <div className="divide-(--kumo-line) scrollbar-slim mt-2 max-h-64 divide-y overflow-y-auto">
          {items.map((it) => (
            <UploadRow key={it.id} item={it} />
          ))}
        </div>
      )}
    </div>
  );
}
