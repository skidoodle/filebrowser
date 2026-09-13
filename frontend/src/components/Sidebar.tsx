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
  XIcon,
} from "@phosphor-icons/react";
import { Button, Meter, useLinkComponent } from "@cloudflare/kumo";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useEffectEvent, useRef, type ReactNode } from "react";
import { api } from "../api/client";
import { auth } from "../api/auth";
import { formatBytes } from "../lib/format";
import { canWriteIn } from "../lib/permissions";
import { navigate, routePath, useRoute } from "../lib/router";

interface SidebarProps {
  currentDir: string;
  onNavigate: (dir: string) => void;
  mobileOpen?: boolean;
  onMobileClose?: () => void;
}

function getRoleLabel(user?: { admin?: boolean; insecure?: boolean; scope?: string } | null): string {
  if (user?.insecure) return "Insecure mode";
  if (user?.admin) return "Administrator";
  if (user?.scope) return `Scope: /${user.scope}`;
  return "Full access";
}

function SidebarFooter({
  signedIn,
  username,
  role,
  showSignIn,
  currentDir,
  onMobileClose,
  onSignOut,
  used,
  total,
  percent,
  versionLabel,
  versionTitle,
  link: Link,
}: {
  signedIn: boolean;
  username?: string;
  role: string;
  showSignIn: boolean;
  currentDir: string;
  onMobileClose?: () => void;
  onSignOut: () => void;
  used: number;
  total: number;
  percent: number;
  versionLabel: string;
  versionTitle?: string;
  link: ReturnType<typeof useLinkComponent>;
}) {
  return (
    <div className="mt-auto flex flex-col gap-3 px-1 pt-4">
      {showSignIn && (
        <SidebarLink
          link={Link}
          href={routePath({ page: "auth", mode: "login", dir: currentDir })}
          icon={<SignInIcon size={20} weight="fill" />}
          onClick={onMobileClose}
        >
          Sign in
        </SidebarLink>
      )}
      {signedIn && (
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => {
              navigate({ page: "settings", tab: "profile", dir: currentDir }, { replace: true });
              onMobileClose?.();
            }}
            title="Open profile settings"
            className="hover:bg-kumo-tint flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-lg px-2 py-2 text-left"
          >
            <span className="bg-kumo-brand-tint text-kumo-brand flex h-8 w-8 shrink-0 items-center justify-center rounded-full">
              <UserCircleIcon size={22} weight="fill" />
            </span>
            <span className="min-w-0">
              <span className="block truncate text-sm leading-4 font-medium" title={username}>
                {username}
              </span>
              <span className="text-kumo-subtle block truncate text-xs leading-4">{role}</span>
            </span>
          </button>
          <button
            type="button"
            onClick={onSignOut}
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
        <span className="truncate leading-5" title={versionTitle}>
          {versionLabel}
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
  );
}

function SidebarNav({
  atFiles,
  atSettings,
  canWriteThisDir,
  signedIn,
  currentDir,
  onNavigate,
  onMobileClose,
  link: Link,
}: {
  atFiles: boolean;
  atSettings: boolean;
  canWriteThisDir: boolean;
  signedIn: boolean;
  currentDir: string;
  onNavigate: (dir: string) => void;
  onMobileClose?: () => void;
  link: ReturnType<typeof useLinkComponent>;
}) {
  return (
    <>
      <button
        type="button"
        onClick={() => {
          onNavigate(".");
          onMobileClose?.();
        }}
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
            onClick={onMobileClose}
          >
            New folder
          </SidebarLink>
          <SidebarLink
            link={Link}
            href={routePath({ page: "create", kind: "file", dir: currentDir })}
            icon={<FilePlusIcon size={20} weight="fill" className="rotate-90" />}
            onClick={onMobileClose}
          >
            New file
          </SidebarLink>
          <div className="bg-kumo-hairline my-2 h-px" />
        </>
      )}

      {signedIn && (
        <button
          type="button"
          onClick={() => {
            navigate({ page: "settings", tab: "profile", dir: currentDir }, { replace: true });
            onMobileClose?.();
          }}
          className={`hover:bg-kumo-tint flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium ${atSettings ? "bg-kumo-tint" : ""}`}
        >
          <GearIcon size={20} weight="fill" className="text-kumo-subtle" />
          Settings
        </button>
      )}
    </>
  );
}

function SidebarHeader({
  onNavigate,
  onMobileClose,
}: {
  onNavigate: (dir: string) => void;
  onMobileClose?: () => void;
}) {
  return (
    <div className="mb-2 flex items-center justify-between">
      <button
        type="button"
        onClick={() => {
          onNavigate(".");
          onMobileClose?.();
        }}
        title="Go to Files"
        className="hover:bg-kumo-tint flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-lg px-2 py-3 text-left"
      >
        <HardDrivesIcon size={28} weight="fill" className="text-kumo-brand shrink-0" />
        <span className="text-lg font-semibold truncate">File Browser</span>
      </button>
      {onMobileClose && (
        <Button
          type="button"
          variant="ghost"
          shape="square"
          aria-label="Close menu"
          icon={<XIcon size={20} />}
          onClick={onMobileClose}
          className="md:hidden"
        />
      )}
    </div>
  );
}

function useMobileClose(mobileOpen: boolean, route: ReturnType<typeof useRoute>, onMobileClose?: () => void) {
  const handleEscape = useEffectEvent(() => {
    onMobileClose?.();
  });

  useEffect(() => {
    if (!mobileOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") handleEscape();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [mobileOpen]);

  const prevRouteRef = useRef(route);
  useEffect(() => {
    if (prevRouteRef.current !== route) {
      prevRouteRef.current = route;
      onMobileClose?.();
    }
  }, [route, onMobileClose]);
}

export function Sidebar({ currentDir, onNavigate, mobileOpen = false, onMobileClose }: SidebarProps) {
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
  const role = getRoleLabel(me.data);

  useMobileClose(mobileOpen, route, onMobileClose);

  const signOut = () => {
    void auth.logout().then(() => {
      void queryClient.invalidateQueries();
      navigate({ page: "files", dir: currentDir }, { replace: true });
      onMobileClose?.();
    });
  };

  const showSignIn = !me.data?.insecure && !signedIn;
  const versionTitle =
    health.data?.commit && health.data.commit !== "none"
      ? `${health.data.version} (${health.data.commit})`
      : health.data?.version;

  return (
    <>
      {mobileOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 backdrop-blur-xs md:hidden"
          onClick={onMobileClose}
          aria-hidden="true"
        />
      )}
      <aside
        className={`bg-kumo-base text-kumo-default border-r border-kumo-hairline p-3 shrink-0 flex-col overflow-y-auto
          ${mobileOpen ? "fixed inset-y-0 left-0 z-50 flex w-72 max-w-[85vw] shadow-2xl" : "hidden md:flex md:w-60"}
        `}
      >
        <SidebarHeader onNavigate={onNavigate} onMobileClose={onMobileClose} />
        <SidebarNav
          atFiles={atFiles}
          atSettings={atSettings}
          canWriteThisDir={canWriteThisDir}
          signedIn={signedIn}
          currentDir={currentDir}
          onNavigate={onNavigate}
          onMobileClose={onMobileClose}
          link={Link}
        />

        <SidebarFooter
          signedIn={signedIn}
          username={me.data?.username}
          role={role}
          showSignIn={showSignIn}
          currentDir={currentDir}
          onMobileClose={onMobileClose}
          onSignOut={signOut}
          used={used}
          total={total}
          percent={percent}
          versionLabel={health.data?.version ?? "dev"}
          versionTitle={versionTitle}
          link={Link}
        />
      </aside>
    </>
  );
}

function SidebarLink({
  link: Link,
  href,
  icon,
  children,
  onClick,
}: {
  link: ReturnType<typeof useLinkComponent>;
  href: string;
  icon: ReactNode;
  children: ReactNode;
  onClick?: () => void;
}) {
  return (
    <Link
      href={href}
      onClick={onClick}
      className="hover:bg-kumo-tint flex items-center gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium text-kumo-default"
    >
      <span className="text-kumo-subtle flex">{icon}</span>
      {children}
    </Link>
  );
}
