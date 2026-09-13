import { Button, Input } from "@cloudflare/kumo";
import { CheckIcon, SignOutIcon, UserCircleIcon } from "@phosphor-icons/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type SubmitEvent } from "react";
import { auth } from "../../api/auth";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/useMe";

function getProfileRole(user?: { insecure?: boolean; admin?: boolean; scope?: string } | null): string {
  if (user?.insecure) return "Insecure mode";
  if (user?.admin) return "Administrator";
  if (user?.scope) return `Scope: /${user.scope}`;
  return "Full access";
}

function ProfileHeader({
  username,
  role,
  insecure,
}: {
  username?: string;
  role: string;
  insecure?: boolean;
}) {
  const queryClient = useQueryClient();

  const handleSignOut = () => {
    void auth.logout().then(() => {
      void queryClient.invalidateQueries();
      navigate({ page: "files", dir: "." }, { replace: true });
    });
  };

  return (
    <div className="bg-kumo-base ring-kumo-hairline flex items-center justify-between gap-4 rounded-xl p-5 ring-1">
      <div className="flex min-w-0 items-center gap-4">
        <span className="bg-kumo-brand-tint text-kumo-brand flex h-12 w-12 shrink-0 items-center justify-center rounded-full">
          <UserCircleIcon size={28} weight="fill" />
        </span>
        <div className="min-w-0">
          <p className="truncate text-base font-semibold">{username ?? "—"}</p>
          <p className="text-kumo-subtle truncate text-sm" title={role}>{role}</p>
        </div>
      </div>
      {!insecure && (
        <Button
          type="button"
          variant="secondary"
          icon={<SignOutIcon size={18} />}
          onClick={handleSignOut}
          className="shrink-0"
        >
          Sign out
        </Button>
      )}
    </div>
  );
}

function ChangeUsernameForm({ initialUsername }: { initialUsername?: string }) {
  const queryClient = useQueryClient();
  const [username, setUsername] = useState(initialUsername ?? "");

  const rename = useMutation({
    mutationFn: () => auth.changeUsername(username.trim()),
    onSuccess: () => {
      void queryClient.invalidateQueries();
      setUsername("");
    },
  });

  const usernameChanged = username.trim() !== "" && username.trim() !== (initialUsername ?? "");
  const usernameValid = /^[a-zA-Z0-9._-]{1,64}$/.test(username.trim());

  return (
    <form
      className="bg-kumo-base ring-kumo-hairline rounded-xl p-5 ring-1"
      onSubmit={(e) => {
        e.preventDefault();
        if (usernameValid && usernameChanged && !rename.isPending) {
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
        <Button
          type="submit"
          variant="secondary"
          loading={rename.isPending}
          disabled={!usernameChanged || !usernameValid}
          className="shrink-0"
        >
          Rename
        </Button>
      </div>
      {rename.error instanceof Error && <p className="text-kumo-danger mt-2 text-sm">{rename.error.message}</p>}
    </form>
  );
}

function ChangePasswordForm() {
  const queryClient = useQueryClient();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [done, setDone] = useState<string | null>(null);

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

  return (
    <form onSubmit={onSubmit} className="bg-kumo-base ring-kumo-hairline rounded-xl p-5 ring-1">
      <h2 className="mb-1 text-base font-semibold">Change password</h2>
      <p className="text-kumo-subtle mb-4 text-sm">Choose a password of at least 8 characters.</p>
      <div className="flex max-w-sm flex-col gap-3">
        <Input
          type="password"
          autoComplete="current-password"
          placeholder="Current password"
          value={current}
          onValueChange={setCurrent}
        />
        <Input
          type="password"
          autoComplete="new-password"
          placeholder="New password"
          value={password}
          onValueChange={setPassword}
        />
        <Input
          type="password"
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
  );
}

export function ProfilePanel() {
  const me = useMe();
  const role = getProfileRole(me.data);

  return (
    <div className="flex flex-col gap-4">
      <ProfileHeader
        username={me.data?.username}
        role={role}
        insecure={me.data?.insecure}
      />

      {me.data?.insecure ? (
        <div className="bg-kumo-base ring-kumo-hairline rounded-xl p-5 ring-1">
          <p className="text-kumo-subtle text-sm">
            Authentication is disabled in insecure mode. Profile and credential settings are not available.
          </p>
        </div>
      ) : (
        <>
          <ChangeUsernameForm initialUsername={me.data?.username} />
          <ChangePasswordForm />
        </>
      )}
    </div>
  );
}
