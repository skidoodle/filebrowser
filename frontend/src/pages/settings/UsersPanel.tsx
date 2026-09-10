import { Badge, Button, Checkbox, Dialog, Input, Loader } from "@cloudflare/kumo";
import { LockKeyIcon, PencilSimpleIcon, PlusIcon, TrashSimpleIcon, UserCircleIcon, XIcon } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { users, type User } from "../../api/auth";

type FormTarget = "new" | User | null;

export function UsersPanel() {
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
