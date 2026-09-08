import {
  FilePlusIcon,
  FolderOpenIcon,
  FolderPlusIcon,
  GearIcon,
  HardDrivesIcon,
  HeadsetIcon,
  SignInIcon,
  SignOutIcon,
  UserCircleIcon,
} from "@phosphor-icons/react";
import { Meter, useLinkComponent } from "@cloudflare/kumo";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../api/client";
import { auth } from "../api/auth";
import { formatBytes } from "../lib/format";
import { canWriteIn } from "../lib/permissions";
import { navigate, routePath, useRoute } from "../lib/router";

interface SidebarProps {
  currentDir: string;
  onNavigate: (dir: string) => void;
}

export function Sidebar({ currentDir, onNavigate }: SidebarProps) {
  const Link = useLinkComponent();
  const route = useRoute();
  const queryClient = useQueryClient();
  const usage = useQuery({ queryKey: ["usage"], queryFn: api.usage, staleTime: 60_000 });
  const health = useQuery({ queryKey: ["health"], queryFn: api.health, staleTime: Infinity });
  const me = useQuery({ queryKey: ["me"], queryFn: auth.me, staleTime: 60_000 });
  const signedIn = me.data?.insecure === true || !!me.data?.username;
  const canWriteThisDir = canWriteIn(me.data, currentDir === "" ? "." : currentDir);

  const atSettings = route.page === "settings";
  const atFiles = !atSettings;

  const used = usage.data?.used ?? 0;
  const total = usage.data?.total ?? 0;
  const percent = total > 0 ? Math.min(100, (used / total) * 100) : 0;

  const role = me.data?.insecure
    ? "Insecure mode"
    : me.data?.admin
      ? "Administrator"
      : me.data?.scope
        ? `Scope: /${me.data.scope}`
        : "Full access";

  const signOut = () => {
    void auth.logout().then(() => {
      void queryClient.invalidateQueries();
      navigate({ page: "files", dir: currentDir }, { replace: true });
    });
  };

  return (
    <aside className="bg-kumo-base text-kumo-default hidden w-60 shrink-0 flex-col border-r border-kumo-hairline p-3 md:flex">
      <button
        type="button"
        onClick={() => onNavigate(".")}
        title="Go to Files"
        className="hover:bg-kumo-tint mb-2 flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-3 text-left"
      >
        <HardDrivesIcon size={28} weight="fill" className="text-kumo-brand" />
        <span className="text-lg font-semibold">filebrowser</span>
      </button>

      <button
        type="button"
        onClick={() => onNavigate(".")}
        className={`hover:bg-kumo-tint flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium ${atFiles ? "bg-kumo-tint" : ""}`}
      >
        <FolderOpenIcon size={20} weight="fill" className="text-kumo-subtle" />
        Files
      </button>

      <div className="bg-kumo-hairline my-2 h-px" />

      {canWriteThisDir && (
        <>
          <SidebarLink
            link={Link}
            href={routePath({ page: "create", kind: "dir", dir: currentDir })}
            icon={<FolderPlusIcon size={20} weight="fill" />}
          >
            New folder
          </SidebarLink>
          <SidebarLink
            link={Link}
            href={routePath({ page: "create", kind: "file", dir: currentDir })}
            icon={<FilePlusIcon size={20} weight="fill" className="rotate-90" />}
          >
            New file
          </SidebarLink>
          <div className="bg-kumo-hairline my-2 h-px" />
        </>
      )}

      {signedIn && (
        <button
          type="button"
          onClick={() => navigate({ page: "settings", tab: "profile", dir: currentDir }, { replace: true })}
          className={`hover:bg-kumo-tint flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium ${atSettings ? "bg-kumo-tint" : ""}`}
        >
          <GearIcon size={20} weight="fill" className="text-kumo-subtle" />
          Settings
        </button>
      )}

      <div className="mt-auto flex flex-col gap-3 px-1">
        {!me.data?.insecure && !signedIn && (
          <SidebarLink
            link={Link}
            href={routePath({ page: "auth", mode: "login", dir: currentDir })}
            icon={<SignInIcon size={20} weight="fill" />}
          >
            Sign in
          </SidebarLink>
        )}
        {signedIn && (
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => navigate({ page: "settings", tab: "profile", dir: currentDir }, { replace: true })}
              title="Open profile settings"
              className="hover:bg-kumo-tint flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-lg px-2 py-2 text-left"
            >
              <span className="bg-kumo-brand-tint text-kumo-brand flex h-8 w-8 shrink-0 items-center justify-center rounded-full">
                <UserCircleIcon size={22} weight="fill" />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm leading-4 font-medium" title={me.data?.username}>
                  {me.data?.username}
                </span>
                <span className="text-kumo-subtle block truncate text-xs leading-4">{role}</span>
              </span>
            </button>
            <button
              type="button"
              onClick={signOut}
              title="Sign out"
              aria-label="Sign out"
              className="text-kumo-subtle hover:bg-kumo-tint hover:text-kumo-danger flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-lg"
            >
              <SignOutIcon size={18} weight="fill" />
            </button>
          </div>
        )}
        <Meter
          label={total > 0 ? `${formatBytes(used)} of ${formatBytes(total)} used` : "Storage usage unavailable"}
          value={percent}
          customValue=""
        />
        <div className="text-kumo-subtle flex items-center justify-between text-xs">
          <span
            className="truncate leading-5"
            title={
              health.data?.commit && health.data.commit !== "none"
                ? `${health.data.version} (${health.data.commit})`
                : health.data?.version
            }
          >
            {health.data?.version ?? "dev"}
          </span>
          <a
            href="https://github.com/skidoodle/filebrowser"
            target="_blank"
            rel="noreferrer"
            className="hover:text-kumo-default flex shrink-0 items-center gap-1"
          >
            <HeadsetIcon size={14} />
            Help
          </a>
        </div>
      </div>
    </aside>
  );
}

function SidebarLink({
  link: Link,
  href,
  icon,
  children,
}: {
  link: ReturnType<typeof useLinkComponent>;
  href: string;
  icon: ReactNode;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className="hover:bg-kumo-tint flex items-center gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium text-kumo-default"
    >
      <span className="text-kumo-subtle flex">{icon}</span>
      {children}
    </Link>
  );
}
