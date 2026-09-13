import { Badge, Button } from "@cloudflare/kumo";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { auth, type AccessPolicy } from "../../api/auth";
import { useMe } from "../../lib/useMe";

export function PolicyPanel() {
  const queryClient = useQueryClient();
  const me = useMe();
  const currentPolicy: AccessPolicy = me.data?.access_policy ?? "public";
  const [override, setOverride] = useState<AccessPolicy | null>(null);

  const selected = override ?? currentPolicy;

  const save = useMutation({
    mutationFn: () => auth.setAccessPolicy(selected),
    onSuccess: () => {
      void queryClient.invalidateQueries();
      setOverride(null);
    },
  });

  const options: {
    id: AccessPolicy;
    title: string;
    description: string;
    badge?: string;
  }[] = [
      {
        id: "public",
        title: "Public",
        description:
          "Anonymous visitors can browse, download, upload files, and create folders.",
        badge: "Default",
      },
      {
        id: "readonly",
        title: "Read-only",
        description:
          "Anonymous visitors can only browse and download files.",
      },
      {
        id: "private",
        title: "Private",
        description:
          "Visitors must sign in to view or modify files.",
      },
    ];

  const changed = selected !== currentPolicy;

  if (me.data?.insecure) {
    return (
      <div className="flex flex-col gap-4">
        <div className="bg-kumo-base ring-kumo-hairline flex flex-col gap-1 rounded-xl p-5 ring-1">
          <h2 className="text-base font-semibold">Access Policy</h2>
          <p className="text-kumo-subtle text-sm">
            Access policy is inactive because filebrowser is running in insecure mode. All visitors have full write access without authentication.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="bg-kumo-base ring-kumo-hairline flex flex-col gap-1 rounded-xl p-5 ring-1">
        <h2 className="text-base font-semibold">Access Policy</h2>
        <p className="text-kumo-subtle text-sm">
          Decide how people without an account can use filebrowser.
        </p>
      </div>

      <div className="flex flex-col gap-3">
        {options.map((opt) => {
          const isSelected = selected === opt.id;
          const isActive = opt.id === currentPolicy;
          return (
            <label
              key={opt.id}
              className={`bg-kumo-base ring-kumo-hairline flex min-h-22 cursor-pointer items-start gap-4 rounded-xl p-5 ring-1 transition-colors ${isSelected
                ? "ring-kumo-brand ring-2 bg-kumo-brand-tint/10"
                : "hover:bg-kumo-tint/20"
                }`}
            >
              <input
                type="radio"
                name="access_policy"
                aria-label={opt.title}
                checked={isSelected}
                onChange={() => setOverride(opt.id)}
                className="mt-1 cursor-pointer accent-kumo-brand"
              />
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <div className="flex min-h-6 items-center justify-between gap-2 flex-wrap">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-sm font-semibold">{opt.title}</span>
                    {opt.badge && (
                      <Badge variant="secondary" className="shrink-0">
                        {opt.badge}
                      </Badge>
                    )}
                  </div>
                  {isActive && (
                    <Badge variant="primary" className="shrink-0">
                      Active
                    </Badge>
                  )}
                </div>
                <p className="text-kumo-subtle text-sm leading-relaxed">
                  {opt.description}
                </p>
              </div>
            </label>
          );
        })}
      </div>

      {save.error instanceof Error && (
        <p className="text-kumo-danger text-sm">{save.error.message}</p>
      )}

      <div className="flex justify-end pt-2">
        <Button
          variant="primary"
          loading={save.isPending}
          disabled={!changed}
          onClick={() => save.mutate()}
        >
          Save Changes
        </Button>
      </div>
    </div>
  );
}
