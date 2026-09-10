import { Badge, Button, Input, Loader, Switch } from "@cloudflare/kumo";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { system, type SystemDynamicSettings } from "../../api/system";
import { formatBytes, parseHumanBytes } from "../../lib/format";

export function SystemPanel() {
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
        Failed to load system settings.
      </div>
    );
  }

  return <SystemSettingsForm initial={data.dynamic} />;
}

export function SystemSettingsForm({ initial }: { initial: SystemDynamicSettings }) {
  const queryClient = useQueryClient();
  const [guard, setGuard] = useState(initial.guard);
  const [maxUpload, setMaxUpload] = useState(formatBytes(initial.max_upload));
  const [maxTextSize, setMaxTextSize] = useState(formatBytes(initial.max_text_size));
  const [requestRate, setRequestRate] = useState(initial.request_rate);
  const [downloadRate, setDownloadRate] = useState(formatBytes(initial.download_rate));
  const [powDifficulty, setPowDifficulty] = useState(initial.pow_difficulty);
  const [trustedProxies, setTrustedProxies] = useState(initial.trusted_proxies);

  const parsedMaxUpload = parseHumanBytes(maxUpload, initial.max_upload);
  const parsedMaxText = parseHumanBytes(maxTextSize, initial.max_text_size);
  const parsedDownloadRate = parseHumanBytes(downloadRate, initial.download_rate);

  const isDirty =
    guard !== initial.guard ||
    parsedMaxUpload !== initial.max_upload ||
    parsedMaxText !== initial.max_text_size ||
    requestRate !== initial.request_rate ||
    parsedDownloadRate !== initial.download_rate ||
    powDifficulty !== initial.pow_difficulty ||
    trustedProxies !== initial.trusted_proxies;

  const save = useMutation({
    mutationFn: async () => {
      await system.update({
        guard,
        max_upload: parsedMaxUpload,
        max_text_size: parsedMaxText,
        request_rate: requestRate,
        download_rate: parsedDownloadRate,
        pow_difficulty: powDifficulty,
        trusted_proxies: trustedProxies,
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["systemSettings"] });
    },
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="bg-kumo-base ring-kumo-hairline flex flex-col gap-1 rounded-xl p-5 ring-1">
        <h2 className="text-base font-semibold">System Configuration</h2>
        <p className="text-kumo-subtle text-sm">
          Dynamic runtime limits and abuse controls applied instantly without server restart.
        </p>
      </div>

      <div className="flex flex-col gap-3">
        <div className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-semibold">Abuse Guard</span>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Enable rate limiting, proof-of-work challenges, and bot mitigation pipelines.
            </p>
          </div>
          <div className="shrink-0">
            <Switch checked={guard} onCheckedChange={(c) => setGuard(c === true)} />
          </div>
        </div>

        <div className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">Maximum Upload Size</span>
              <Badge variant="secondary">{formatBytes(parsedMaxUpload)}</Badge>
            </div>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Upper bound for upload streams and chunked uploads (e.g. 10GiB, 500MiB).
            </p>
          </div>
          <div className="w-full sm:w-48 shrink-0">
            <Input
              value={maxUpload}
              onChange={(e) => setMaxUpload(e.target.value)}
              placeholder="e.g. 10GiB"
            />
          </div>
        </div>

        <div className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">Text Preview Size</span>
              <Badge variant="secondary">{formatBytes(parsedMaxText)}</Badge>
            </div>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Maximum file size rendered into the built-in text and code viewer (e.g. 10MiB).
            </p>
          </div>
          <div className="w-full sm:w-48 shrink-0">
            <Input
              value={maxTextSize}
              onChange={(e) => setMaxTextSize(e.target.value)}
              placeholder="e.g. 10MiB"
            />
          </div>
        </div>

        <div className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">API Request Rate</span>
              <Badge variant="secondary">{requestRate} req/s</Badge>
            </div>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Per-IP request budget before rate limiting throttles requests.
            </p>
          </div>
          <div className="w-full sm:w-48 shrink-0">
            <Input
              type="number"
              min={1}
              value={requestRate}
              onChange={(e) => setRequestRate(Math.max(1, parseInt(e.target.value, 10) || 1))}
            />
          </div>
        </div>

        <div className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">Download Rate Limit</span>
              <Badge variant="secondary">{formatBytes(parsedDownloadRate)}/s</Badge>
            </div>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Per-IP download bandwidth throttle for raw file streams (e.g. 200MiB).
            </p>
          </div>
          <div className="w-full sm:w-48 shrink-0">
            <Input
              value={downloadRate}
              onChange={(e) => setDownloadRate(e.target.value)}
              placeholder="e.g. 200MiB"
            />
          </div>
        </div>

        <div className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">Proof-of-Work Difficulty</span>
              <Badge variant="secondary">{powDifficulty === 0 ? "Disabled" : `${powDifficulty} hex zeros`}</Badge>
            </div>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Leading zero count required in proof-of-work challenges for anonymous mutations (0 disables).
            </p>
          </div>
          <div className="w-full sm:w-48 shrink-0">
            <Input
              type="number"
              min={0}
              max={16}
              value={powDifficulty}
              onChange={(e) => setPowDifficulty(Math.min(16, Math.max(0, parseInt(e.target.value, 10) || 0)))}
            />
          </div>
        </div>

        <div className="bg-kumo-base ring-kumo-hairline flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">Trusted Reverse Proxies</span>
            </div>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Comma-separated CIDRs (e.g. 127.0.0.1/32, 10.0.0.0/8) whose X-Forwarded-For headers are trusted.
            </p>
          </div>
          <div className="w-full sm:w-48 shrink-0">
            <Input
              value={trustedProxies}
              onChange={(e) => setTrustedProxies(e.target.value)}
              placeholder="127.0.0.1/32, 10.0.0.0/8"
            />
          </div>
        </div>
      </div>

      {save.error instanceof Error && (
        <p className="text-kumo-danger text-sm">{save.error.message}</p>
      )}

      <div className="flex justify-end pt-2">
        <Button
          variant="primary"
          loading={save.isPending}
          disabled={!isDirty}
          onClick={() => save.mutate()}
        >
          Save Changes
        </Button>
      </div>
    </div>
  );
}
