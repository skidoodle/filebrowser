import { Badge, Loader } from "@cloudflare/kumo";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { system } from "../../api/system";
import { formatUptime } from "../../lib/format";

function DynamicUptimeBadge({ initialSeconds }: { initialSeconds: number }) {
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    const start = performance.now();
    const timer = setInterval(() => {
      setElapsed(Math.floor((performance.now() - start) / 1000));
    }, 1000);
    return () => clearInterval(timer);
  }, [initialSeconds]);

  return <Badge variant="secondary">{formatUptime(initialSeconds + elapsed)}</Badge>;
}

export function AboutPanel() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["systemSettings"],
    queryFn: system.get,
  });

  if (isLoading) {
    return (
      <div className="flex h-48 items-center justify-center">
        <Loader size="lg" />
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="bg-kumo-base ring-kumo-hairline rounded-xl p-5 ring-1 text-kumo-danger text-sm">
        Failed to load host information.
      </div>
    );
  }

  const { info } = data;

  interface InfoRow {
    title: string;
    desc: string;
    value?: string;
    badge?: string;
    component?: React.ReactNode;
    mono?: boolean;
  }

  const rows: InfoRow[] = [
    {
      title: "Storage Root",
      desc: "Directory on the host served to users",
      value: info.root,
      mono: true,
    },
    {
      title: "Database File",
      desc: "Location of the persistent SQLite database",
      value: info.database,
      mono: true,
    },
    {
      title: "Cache Directory",
      desc: "Directory for thumbnails and cached assets",
      value: info.cache_dir,
      mono: true,
    },
    {
      title: "Listen Address",
      desc: "Host address and port bound by the HTTP server",
      value: info.address,
      mono: true,
    },
    {
      title: "Base URL Subpath",
      desc: "Mounted path prefix if served behind reverse proxy",
      value: info.base_url || "(root)",
      mono: true,
    },
    {
      title: "Build & Release",
      desc: "Application version and git commit sha",
      badge: `${info.version} (${info.commit || "dev"})`,
    },
    {
      title: "Environment & Runtime",
      desc: "Host operating system, CPU architecture, and Go version",
      badge: `${info.go_version} ${info.os}/${info.arch}`,
    },
    {
      title: "Uptime",
      desc: "Time since the filebrowser server started",
      component: <DynamicUptimeBadge initialSeconds={info.uptime_seconds} />,
    },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="bg-kumo-base ring-kumo-hairline flex flex-col gap-1 rounded-xl p-5 ring-1">
        <h2 className="text-base font-semibold">About Filebrowser</h2>
        <p className="text-kumo-subtle text-sm">
          System specifications, runtime environment, and active file paths.
        </p>
      </div>

      <div className="flex flex-col gap-3">
        {rows.map((row) => (
          <div
            key={row.title}
            className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-xl p-5 ring-1"
          >
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-semibold">{row.title}</span>
              <span className="text-kumo-subtle text-xs leading-relaxed">{row.desc}</span>
            </div>
            <div className="shrink-0">
              {row.component ? (
                row.component
              ) : row.badge ? (
                <Badge variant="secondary">{row.badge}</Badge>
              ) : row.mono ? (
                <code className="text-xs bg-kumo-tint/20 text-kumo-default px-2.5 py-1 rounded-md font-mono break-all max-w-sm block">
                  {row.value}
                </code>
              ) : (
                <span className="text-sm">{row.value}</span>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
