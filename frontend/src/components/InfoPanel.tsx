import { DownloadSimpleIcon, FilesIcon } from "@phosphor-icons/react";
import { Button } from "@cloudflare/kumo";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { formatBytes, formatRelative } from "../lib/format";
import { FileTypeIcon } from "../lib/icons";
import { usePrefs } from "../stores/prefs";

const CHECKSUM_ALGOS = ["md5", "sha256"] as const;
const panelShell = "border-kumo-hairline hidden w-72 shrink-0 overflow-y-auto border-l p-5 xl:block";

interface InfoPanelProps {
  paths: string[];
  dir: string;
}

export function InfoPanel({ paths, dir }: InfoPanelProps) {
  const single = paths.length === 1 ? paths[0] : null;

  const meta = useQuery({
    queryKey: ["meta", single],
    queryFn: () => api.meta(single!),
    enabled: single !== null,
  });

  if (paths.length === 0) {
    return (
      <aside className="border-kumo-hairline hidden w-72 shrink-0 border-l xl:block">
        <p className="text-kumo-subtle p-5 text-sm">Select an entry to see its details.</p>
      </aside>
    );
  }

  if (single !== null) {
    return <SingleInfo path={single} meta={meta.data} fetching={meta.isFetching} />;
  }

  return <MultiSummary paths={paths} dir={dir} />;
}

function SingleInfo({
  path,
  meta,
  fetching,
}: {
  path: string;
  meta:
  | import("../types").FileMeta
  | undefined;
  fetching: boolean;
}) {
  const [algo, setAlgo] = useState<(typeof CHECKSUM_ALGOS)[number] | null>(null);

  const checksum = useQuery({
    queryKey: ["meta", path, algo],
    queryFn: () => api.meta(path, algo ?? undefined),
    enabled: algo !== null,
  });

  if (!meta) {
    return (
      <aside className="border-kumo-hairline hidden w-72 shrink-0 border-l p-5 xl:block">
        <p className="text-kumo-subtle text-sm">Loading…</p>
      </aside>
    );
  }

  const file = meta;
  const shown = algo !== null ? (checksum.data?.checksums?.[algo] ?? (fetching ? "Computing…" : "–")) : null;

  return (
    <aside className={panelShell}>
      <div className="flex flex-col gap-5">
        {file.type === "image" && (
          <img
            src={api.bigUrl(file.path)}
            alt={file.name}
            draggable={false}
            className="ring-kumo-hairline max-h-48 w-full rounded-lg object-contain ring-1 pointer-events-none select-none"
          />
        )}

        <div className="flex items-center gap-3">
          <FileTypeIcon type={file.type} size={36} />
          <p className="min-w-0 flex-1 truncate leading-6 text-sm font-semibold" title={file.name}>
            {file.name}
          </p>
        </div>

        {!file.isDir && (
          <Button
            variant="secondary"
            icon={<DownloadSimpleIcon size={16} />}
            onClick={() => window.location.assign(api.rawUrl(file.path))}
          >
            Download
          </Button>
        )}

        <dl className="flex flex-col gap-4">
          <Row label="Type" value={file.mime ?? file.type} />
          {!file.isDir && <Row label="Size" value={formatBytes(file.size)} />}
          <Row label="Modified" value={formatRelative(file.modified)} />
          {file.extension && <Row label="Extension" value={`.${file.extension}`} />}
          {file.isDir || <Row label="Path" value={file.path} mono />}
        </dl>

        {!file.isDir && (
          <div className="flex flex-col gap-2">
            <p className="text-kumo-subtle text-sm font-medium">Checksums</p>
            <div className="flex gap-1.5">
              {CHECKSUM_ALGOS.map((a) => (
                <Button
                  key={a}
                  size="sm"
                  variant={algo === a ? "primary" : "secondary"}
                  onClick={() => setAlgo(algo === a ? null : a)}
                >
                  {a.toUpperCase()}
                </Button>
              ))}
            </div>
            {algo && (
              <p className="ring-kumo-hairline bg-kumo-base rounded-lg p-2.5 font-mono text-xs break-all ring-1">
                {shown}
              </p>
            )}
          </div>
        )}
      </div>
    </aside>
  );
}

function MultiSummary({ paths, dir }: { paths: string[]; dir: string }) {
  const prefs = usePrefs();
  const list = useQuery({
    queryKey: ["list", dir, prefs.sortBy, prefs.sortOrder],
    queryFn: () => api.list(dir, prefs.sortBy, prefs.sortOrder),
  });

  const pathSet = new Set(paths);
  const items = (list.data?.items ?? []).filter((i) => pathSet.has(i.path));
  const dirs = items.filter((i) => i.isDir);
  const files = items.filter((i) => !i.isDir);
  const totalSize = items.reduce((acc, i) => acc + i.size, 0);

  const typeCounts = new Map<string, number>();
  for (const f of files) {
    const t = f.type.charAt(0).toUpperCase() + f.type.slice(1);
    typeCounts.set(t, (typeCounts.get(t) ?? 0) + 1);
  }

  const names = paths.map((p) => p.split("/").pop() ?? p);

  return (
    <aside className={panelShell}>
      <div className="flex flex-col gap-5">
        <div className="flex items-center gap-3">
          <FilesIcon size={36} weight="fill" className="text-kumo-info" />
          <div className="min-w-0">
            <p className="text-sm font-semibold">{paths.length} items selected</p>
            <p className="text-kumo-subtle text-xs">Selection summary</p>
          </div>
        </div>

        <Button
          variant="secondary"
          icon={<DownloadSimpleIcon size={16} />}
          onClick={() => window.location.assign(api.downloadUrl(dir, names))}
        >
          Download as zip
        </Button>

        <dl className="flex flex-col gap-4">
          <Row label="Folders" value={String(dirs.length)} />
          <Row label="Files" value={String(files.length)} />
          <Row label="Total size" value={formatBytes(totalSize)} />
          {[...typeCounts.entries()].map(([type, count]) => (
            <Row key={type} label={`${type} files`} value={String(count)} />
          ))}
        </dl>

        <p className="text-kumo-subtle text-xs">
          Select a single entry to inspect its properties.
        </p>
      </div>
    </aside>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="grid gap-1">
      <dt className="text-kumo-subtle text-sm">{label}</dt>
      <dd className={`text-sm ${mono ? "font-mono text-xs break-all" : "wrap-break-word"}`} title={value}>
        {value}
      </dd>
    </div>
  );
}
