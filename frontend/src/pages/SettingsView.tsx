import { Badge, Button, Checkbox, Dialog, Input, Loader, Switch } from "@cloudflare/kumo";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeftIcon,
  CheckIcon,
  InfoIcon,
  KeyIcon,
  ListIcon,
  LockKeyIcon,
  PencilSimpleIcon,
  PlusIcon,
  ShieldCheckIcon,
  SignOutIcon,
  SlidersHorizontalIcon,
  TrashSimpleIcon,
  UserCircleIcon,
  XIcon,
} from "@phosphor-icons/react";
import { useState, useEffect, type SubmitEvent } from "react";
import { users, auth, type User, type AccessPolicy } from "../api/auth";
import { system, type SystemDynamicSettings } from "../api/system";
import { formatBytes, formatUptime, parseHumanBytes } from "../lib/format";
import { useMe } from "../lib/useMe";
import { navigate } from "../lib/router";

export type SettingsTab = "profile" | "users" | "policy" | "system" | "about";

interface SettingsViewProps {
  tab: SettingsTab;
  onTabChange: (tab: SettingsTab) => void;
  onClose: () => void;
  onOpenMobileMenu?: () => void;
}

export function SettingsView({ tab, onTabChange, onClose, onOpenMobileMenu }: SettingsViewProps) {
  const me = useMe();
  const admin = me.data?.admin === true;

  const tabs: { id: SettingsTab; label: string; icon: React.ReactNode; adminOnly?: boolean }[] = [
    { id: "profile", label: "Profile", icon: <KeyIcon size={16} /> },
    { id: "users", label: "Users", icon: <ShieldCheckIcon size={16} />, adminOnly: true },
    { id: "policy", label: "Access Policy", icon: <LockKeyIcon size={16} />, adminOnly: true },
    { id: "system", label: "System", icon: <SlidersHorizontalIcon size={16} />, adminOnly: true },
    { id: "about", label: "About", icon: <InfoIcon size={16} />, adminOnly: true },
  ];

  return (
    <main className="bg-kumo-canvas text-kumo-default min-w-0 flex-1 overflow-y-auto scrollbar-gutter-stable">
      <div className="mx-auto w-full max-w-3xl p-4 md:p-8">
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Button variant="ghost" shape="square" aria-label="Back to files" icon={<ArrowLeftIcon size={20} />} onClick={onClose} />
            <h1 className="text-xl font-semibold">Settings</h1>
          </div>
          {onOpenMobileMenu && (
            <Button
              type="button"
              variant="ghost"
              shape="square"
              aria-label="Open menu"
              icon={<ListIcon size={20} />}
              onClick={onOpenMobileMenu}
              className="md:hidden"
            />
          )}
        </div>

        <div className="border-kumo-hairline mb-6 flex gap-1 border-b">
          {tabs
            .filter((t) => !t.adminOnly || admin)
            .map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => onTabChange(t.id)}
                className={`-mb-px flex cursor-pointer items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${tab === t.id
                  ? "border-kumo-brand text-kumo-brand"
                  : "text-kumo-subtle hover:text-kumo-default border-transparent"
                  }`}
              >
                {t.icon}
                {t.label}
              </button>
            ))}
        </div>

        {tab === "profile" ? (
          <ProfilePanel />
        ) : tab === "users" ? (
          <UsersPanel />
        ) : tab === "policy" ? (
          <PolicyPanel />
        ) : tab === "system" ? (
          <SystemPanel />
        ) : (
          <AboutPanel />
        )}
      </div>
    </main>
  );
}

function ProfilePanel() {
  const me = useMe();
  const queryClient = useQueryClient();
  const [username, setUsername] = useState(me.data?.username ?? "");
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [done, setDone] = useState<string | null>(null);

  const rename = useMutation({
    mutationFn: () => auth.changeUsername(username.trim()),
    onSuccess: () => {
      void queryClient.invalidateQueries();
      setUsername("");
    },
  });

  const change = useMutation({
    mutationFn: () => auth.changePassword(current, password),
    onSuccess: () => {
      void queryClient.invalidateQueries();
      setDone("Password updated.");
      setCurrent("");
      setPassword("");
      setConfirm("");
    },
  });

  const usernameChanged = username.trim() !== "" && username.trim() !== (me.data?.username ?? "");
  const usernameValid = /^[a-zA-Z0-9._-]{1,64}$/.test(username.trim());

  const mismatch = confirm.length > 0 && password !== confirm;
  const tooShort = password.length > 0 && password.length < 8;
  const disabled = !current || !password || password !== confirm || password.length < 8 || change.isPending;

  const onSubmit = (e: SubmitEvent) => {
    e.preventDefault();
    if (!disabled) {
      setDone(null);
      change.mutate();
    }
  };

  const role = me.data?.admin ? "Administrator" : me.data?.scope ? `Scoped to /${me.data.scope}` : "Full access";

  return (
    <div className="flex flex-col gap-4">
      <div className="bg-kumo-base ring-kumo-hairline flex items-center justify-between gap-4 rounded-xl p-5 ring-1">
        <div className="flex min-w-0 items-center gap-4">
          <span className="bg-kumo-brand-tint text-kumo-brand flex h-12 w-12 shrink-0 items-center justify-center rounded-full">
            <UserCircleIcon size={28} weight="fill" />
          </span>
          <div className="min-w-0">
            <p className="truncate text-base font-semibold">{me.data?.username ?? "—"}</p>
            <p className="text-kumo-subtle text-sm">{role}</p>
          </div>
        </div>
        {!me.data?.insecure && (
          <Button
            type="button"
            variant="secondary"
            icon={<SignOutIcon size={18} />}
            onClick={() => {
              void auth.logout().then(() => {
                void queryClient.invalidateQueries();
                navigate({ page: "files", dir: "." }, { replace: true });
              });
            }}
            className="shrink-0"
          >
            Sign out
          </Button>
        )}
      </div>

      <form
        className="bg-kumo-base ring-kumo-hairline rounded-xl p-5 ring-1"
        onSubmit={(e) => {
          e.preventDefault();
          if (usernameValid && usernameChanged && !rename.isPending) {
            setDone(null);
            rename.mutate();
          }
        }}
      >
        <h2 className="mb-1 text-base font-semibold">Username</h2>
        <p className="text-kumo-subtle mb-4 text-sm">
          Your sign-in name. Sessions stay signed in; the old name stops working immediately.
        </p>
        <div className="flex max-w-sm items-start gap-2">
          <Input
            type="text"
            autoComplete="username"
            placeholder="New username (letters, digits, . _ -)"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
          <Button type="submit" variant="secondary" loading={rename.isPending} disabled={!usernameChanged || !usernameValid} className="shrink-0">
            Rename
          </Button>
        </div>
        {rename.error instanceof Error && <p className="text-kumo-danger mt-2 text-sm">{rename.error.message}</p>}
      </form>

      <form onSubmit={onSubmit} className="bg-kumo-base ring-kumo-hairline rounded-xl p-5 ring-1">
        <h2 className="mb-1 text-base font-semibold">Change password</h2>
        <p className="text-kumo-subtle mb-4 text-sm">Choose a password of at least 8 characters.</p>
        <div className="flex max-w-sm flex-col gap-3">
          <Input type="password"
            autoComplete="current-password"
            placeholder="Current password"
            value={current}
            onValueChange={setCurrent}
          />
          <Input type="password"
            autoComplete="new-password"
            placeholder="New password"
            value={password}
            onValueChange={setPassword}
          />
          <Input type="password"
            autoComplete="new-password"
            placeholder="Repeat new password"
            value={confirm}
            onValueChange={setConfirm}
          />
          {mismatch && <p className="text-kumo-danger text-sm">Passwords do not match.</p>}
          {tooShort && <p className="text-kumo-danger text-sm">Password must be at least 8 characters.</p>}
          {change.error instanceof Error && <p className="text-kumo-danger text-sm">{change.error.message}</p>}
          {done && (
            <p className="text-kumo-success flex items-center gap-1.5 text-sm">
              <CheckIcon size={16} weight="bold" /> {done}
            </p>
          )}
          <Button type="submit" variant="primary" loading={change.isPending} disabled={disabled} className="mt-1 self-start">
            Update password
          </Button>
        </div>
      </form>
    </div>
  );
}

type FormTarget = "new" | User | null;

function UsersPanel() {
  const queryClient = useQueryClient();
  const [formTarget, setFormTarget] = useState<FormTarget>(null);
  const [deleting, setDeleting] = useState<User | null>(null);

  const list = useQuery({ queryKey: ["users"], queryFn: users.list });

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["users"] });

  const remove = useMutation({
    mutationFn: (id: number) => users.remove(id),
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <div>
          <h2 className="text-base font-semibold">Users</h2>
          <p className="text-kumo-subtle text-sm">
            {list.data ? `${list.data.length} account${list.data.length === 1 ? "" : "s"}` : "Accounts"} on this server
          </p>
        </div>
        <Button variant="primary" className="ml-auto" onClick={() => setFormTarget(formTarget === "new" ? null : "new")}>
          <PlusIcon size={16} weight="bold" className="mr-1" />
          New user
        </Button>
      </div>

      {formTarget === "new" && (
        <UserForm
          key="new"
          user={null}
          onClose={() => setFormTarget(null)}
          onSaved={() => {
            setFormTarget(null);
            invalidate();
          }}
        />
      )}

      {list.isPending && <Loader className="mx-auto mt-10" />}
      {list.isError && <p className="text-kumo-danger">{list.error.message}</p>}

      {list.data && (
        <div className="bg-kumo-base ring-kumo-hairline divide-kumo-hairline divide-y overflow-hidden rounded-xl ring-1">
          {list.data.map((u) => (
            <div key={u.id} className={formTarget !== null && formTarget !== "new" && formTarget.id === u.id ? "bg-kumo-tint/40" : ""}>
              <div className="flex items-center gap-3 p-4">
                <span className="bg-kumo-tint text-kumo-default flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-xs font-semibold uppercase">
                  {u.username.slice(0, 2)}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-1.5 truncate text-sm font-medium">
                    {u.username}
                    {u.isOriginal && (
                      <span title="Original admin — cannot be deleted">
                        <LockKeyIcon size={14} className="text-kumo-subtle" />
                      </span>
                    )}
                  </p>
                  <p className="text-kumo-subtle truncate font-mono text-xs">
                    {u.admin ? "Full access" : u.scope ? `Scope: /${u.scope}` : "No scope set"}
                  </p>
                </div>
                <Badge variant={u.admin ? "primary" : "secondary"}>{u.admin ? "Admin" : "Scoped"}</Badge>
                <div className="flex shrink-0">
                  <Button variant="ghost" shape="square" aria-label={`Edit ${u.username}`} icon={<PencilSimpleIcon size={16} />} onClick={() => setFormTarget(formTarget !== "new" && formTarget?.id === u.id ? null : u)} />
                  <Button
                    variant="ghost"
                    shape="square"
                    aria-label={`Delete ${u.username}`}
                    icon={<TrashSimpleIcon size={16} className={u.isOriginal ? "text-kumo-subtle" : "text-kumo-danger"} />}
                    onClick={() => setDeleting(u)}
                  />
                </div>
              </div>
              {formTarget !== null && formTarget !== "new" && formTarget.id === u.id && (
                <div className="px-4 pb-4">
                  <UserForm
                    key={u.id}
                    user={u}
                    onClose={() => setFormTarget(null)}
                    onSaved={() => {
                      setFormTarget(null);
                      invalidate();
                    }}
                  />
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {deleting !== null && (
        <Dialog.Root
          open
          onOpenChange={(o) => {
            if (!o) setDeleting(null);
          }}
        >
          <Dialog className="p-6 top-20 sm:top-24 z-50">
            <Dialog.Title>Delete user</Dialog.Title>
            <Dialog.Description>
              {`Delete account "${deleting.username}"? Their private folders become public again.`}
            </Dialog.Description>
            {remove.error instanceof Error && <p className="text-kumo-danger mt-3 text-sm">{remove.error.message}</p>}
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setDeleting(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                loading={remove.isPending}
                disabled={deleting.isOriginal}
                onClick={() => remove.mutate(deleting.id)}
              >
                Delete
              </Button>
            </div>
          </Dialog>
        </Dialog.Root>
      )}
    </div>
  );
}

interface UserFormProps {
  user: User | null;
  onClose: () => void;
  onSaved: () => void;
}

function UserForm({ user, onClose, onSaved }: UserFormProps) {
  const [username, setUsername] = useState(user?.username ?? "");
  const [admin, setAdmin] = useState(user?.admin ?? false);
  const [scope, setScope] = useState(user?.scope ?? "");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: async () => {
      setError(null);
      if (user) {
        const patch: { username?: string; admin?: boolean; scope?: string; password?: string } = { admin, scope };
        const trimmed = username.trim();
        if (trimmed !== user.username) patch.username = trimmed;
        if (password) patch.password = password;
        await users.update(user.id, patch);
      } else {
        await users.create({ username: username.trim(), password, admin, scope });
      }
    },
    onSuccess: () => {
      setPassword("");
      setConfirm("");
      setError(null);
      onSaved();
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const usernameChanged = !user || username.trim() !== user.username;
  const mismatch = confirm.length > 0 && password !== confirm;
  const valid =
    (user ? true : /^[a-zA-Z0-9._-]{1,64}$/.test(username.trim())) &&
    (user ? password === "" || password.length >= 8 : password.length >= 8) &&
    password === confirm &&
    (user ? usernameChanged || password !== "" || admin !== user.admin || scope !== user.scope : true);

  return (
    <form
      className="bg-kumo-base ring-kumo-brand/40 rounded-xl p-5 ring-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (valid && !save.isPending) save.mutate();
      }}
    >
      <div className="mb-3 flex items-center gap-2">
        {user ? <PencilSimpleIcon size={18} className="text-kumo-brand" /> : <UserCircleIcon size={18} className="text-kumo-brand" />}
        <h3 className="text-sm font-semibold">{user ? `Edit ${user.username}` : "New user"}</h3>
        <Button type="button" variant="ghost" shape="square" aria-label="Close form" icon={<XIcon size={16} />} className="ml-auto" onClick={onClose} />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Input
          autoFocus
          type="text"
          placeholder="Username (letters, digits, . _ -)"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <Input type="password"
          placeholder={user ? "New password (leave empty to keep)" : "Password (min 8 characters)"}
          value={password}
          onValueChange={setPassword}
        />
        <Input
          type="text"
          placeholder="Scope folder, e.g. docs (empty = whole storage)"
          value={scope}
          disabled={admin}
          onChange={(e) => setScope(e.target.value)}
        />
        <Input type="password"
          placeholder="Repeat password"
          value={confirm}
          onValueChange={setConfirm}
        />
      </div>
      <div className="mt-3">
        <Checkbox
          label="Administrator (full access, user management)"
          checked={admin}
          disabled={user?.isOriginal}
          onCheckedChange={(checked) => setAdmin(checked === true)}
        />
      </div>
      <p className="text-kumo-subtle mt-2 text-xs">
        {admin
          ? "Administrators have access to everything; the scope is ignored."
          : "Scoped users can read everywhere but only modify inside their scope folder."}
      </p>
      {mismatch && <p className="text-kumo-danger mt-2 text-sm">Passwords do not match.</p>}
      {error && <p className="text-kumo-danger mt-2 text-sm">{error}</p>}
      <div className="mt-4 flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" loading={save.isPending} disabled={!valid}>
          {user ? "Save changes" : "Create user"}
        </Button>
      </div>
    </form>
  );
}

function PolicyPanel() {
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
            <div
              key={opt.id}
              onClick={() => setOverride(opt.id)}
              className={`bg-kumo-base ring-kumo-hairline flex min-h-22 cursor-pointer items-start gap-4 rounded-xl p-5 ring-1 transition-colors ${isSelected
                ? "ring-kumo-brand ring-2 bg-kumo-brand-tint/10"
                : "hover:bg-kumo-tint/20"
                }`}
            >
              <input
                type="radio"
                name="access_policy"
                checked={isSelected}
                onChange={() => setOverride(opt.id)}
                className="mt-1 cursor-pointer accent-kumo-brand"
              />
              <div className="flex flex-1 flex-col gap-1">
                <div className="flex min-h-6 items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold">{opt.title}</span>
                    {opt.badge && (
                      <Badge variant="secondary">
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
            </div>
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

function SystemPanel() {
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

function SystemSettingsForm({ initial }: { initial: SystemDynamicSettings }) {
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
    mutationFn: () =>
      system.update({
        guard,
        max_upload: parsedMaxUpload,
        max_text_size: parsedMaxText,
        request_rate: requestRate,
        download_rate: parsedDownloadRate,
        pow_difficulty: powDifficulty,
        trusted_proxies: trustedProxies,
      }),
    onSuccess: (updated) => {
      queryClient.setQueryData(["systemSettings"], updated);
    },
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="bg-kumo-base ring-kumo-hairline flex flex-col gap-1 rounded-xl p-5 ring-1">
        <h2 className="text-base font-semibold">System Configuration</h2>
        <p className="text-kumo-subtle text-sm">
          Dynamic runtime limits and abuse controls. Changes apply immediately without daemon restart.
        </p>
      </div>

      <div className="flex flex-col gap-3">
        <div className="bg-kumo-base ring-kumo-hairline flex items-center justify-between gap-4 rounded-xl p-5 ring-1">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-semibold">Abuse Protection</span>
            <p className="text-kumo-subtle text-sm leading-relaxed">
              Enables active defense: honeypot decoys, progressive IP bans, and rate limits.
            </p>
          </div>
          <Switch checked={guard} onCheckedChange={setGuard} />
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

function AboutPanel() {
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
