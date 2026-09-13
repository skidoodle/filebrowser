import { Button, Input } from "@cloudflare/kumo";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeftIcon, HardDrivesIcon } from "@phosphor-icons/react";
import { useState, type SubmitEvent } from "react";
import { auth } from "../api/auth";
import { navigate, useRoute } from "../lib/router";
import { useMe } from "../lib/useMe";

interface AuthScreenProps {
  mode: "setup" | "login";
}

function SetupForm({ dir }: { dir: string }) {
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const queryClient = useQueryClient();

  const submit = useMutation({
    mutationFn: async () => {
      await auth.setup(password);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      navigate({ page: "files", dir }, { replace: true });
    },
  });

  const mismatch = confirm.length > 0 && password !== confirm;
  const tooShort = password.length > 0 && password.length < 8;
  const disabled = password.length === 0 || confirm !== password || password.length < 8 || submit.isPending;

  const onSubmit = (e: SubmitEvent) => {
    e.preventDefault();
    if (!disabled) submit.mutate();
  };

  return (
    <form onSubmit={onSubmit}>
      <div className="mb-6 flex items-center gap-3">
        <HardDrivesIcon size={28} weight="fill" className="text-kumo-brand" />
        <h1 className="text-lg font-semibold">Create your admin account</h1>
      </div>
      <p className="text-kumo-subtle mb-5 text-sm">
        This server has no accounts yet. Create the admin account to unlock file management (modify, delete, move).
      </p>
      <div className="flex flex-col gap-3">
        <Input
          autoFocus
          type="password"
          value={password}
          autoComplete="new-password"
          placeholder="New password (min 8 characters)"
          onChange={(e) => setPassword(e.target.value)}
        />
        <Input
          type="password"
          value={confirm}
          placeholder="Repeat password"
          onChange={(e) => setConfirm(e.target.value)}
        />
        {mismatch && <p className="text-kumo-danger text-sm">Passwords do not match.</p>}
        {tooShort && <p className="text-kumo-danger text-sm">Password must be at least 8 characters.</p>}
        {submit.error instanceof Error && <p className="text-kumo-danger text-sm">{submit.error.message}</p>}
        <div className="mt-1 flex gap-2">
          <Button
            type="submit"
            variant="primary"
            loading={submit.isPending}
            disabled={disabled}
            className="flex-1"
          >
            Create account
          </Button>
        </div>
      </div>
    </form>
  );
}

function LoginForm({
  dir,
  canGoBack,
  onBack,
  accessPolicy,
}: {
  dir: string;
  canGoBack: boolean;
  onBack: () => void;
  accessPolicy?: string;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const queryClient = useQueryClient();

  const submit = useMutation({
    mutationFn: async () => {
      await auth.login(username, password);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      navigate({ page: "files", dir }, { replace: true });
    },
  });

  const disabled = password.length === 0 || username.trim().length === 0 || submit.isPending;

  const onSubmit = (e: SubmitEvent) => {
    e.preventDefault();
    if (!disabled) submit.mutate();
  };

  return (
    <form onSubmit={onSubmit}>
      <div className="mb-6 flex items-center gap-3">
        {canGoBack && (
          <Button
            type="button"
            variant="ghost"
            shape="square"
            aria-label="Back to files"
            title="Back to files"
            icon={<ArrowLeftIcon size={20} />}
            onClick={onBack}
            disabled={submit.isPending}
          />
        )}
        <HardDrivesIcon size={28} weight="fill" className="text-kumo-brand" />
        <h1 className="text-lg font-semibold">Sign-in</h1>
      </div>
      <p className="text-kumo-subtle mb-5 text-sm">
        {accessPolicy === "private"
          ? "Sign in to access this filebrowser."
          : "Sign in to unlock file management in your scope."}
      </p>
      <div className="flex flex-col gap-3">
        <Input
          autoFocus
          type="text"
          value={username}
          autoComplete="username"
          placeholder="Username"
          onChange={(e) => setUsername(e.target.value)}
        />
        <Input
          type="password"
          value={password}
          autoComplete="current-password"
          placeholder="Password"
          onChange={(e) => setPassword(e.target.value)}
        />
        {submit.error instanceof Error && <p className="text-kumo-danger text-sm">{submit.error.message}</p>}
        <div className="mt-1 flex gap-2">
          {canGoBack && (
            <Button
              type="button"
              variant="secondary"
              onClick={onBack}
              disabled={submit.isPending}
              className="flex-1"
            >
              Back
            </Button>
          )}
          <Button
            type="submit"
            variant="primary"
            loading={submit.isPending}
            disabled={disabled}
            className="flex-1"
          >
            Sign in
          </Button>
        </div>
      </div>
    </form>
  );
}

export function AuthScreen({ mode }: AuthScreenProps) {
  const route = useRoute();
  const me = useMe();

  const canGoBack = mode === "login" && me.data?.access_policy !== "private";

  const onBack = () => {
    navigate({ page: "files", dir: route.dir || "." }, { replace: true });
  };

  return (
    <div className="bg-kumo-canvas flex h-full items-center justify-center p-6">
      <div className="bg-kumo-base ring-kumo-hairline w-full max-w-sm rounded-2xl p-8 shadow-sm ring-1">
        {mode === "setup" ? (
          <SetupForm dir={route.dir} />
        ) : (
          <LoginForm
            dir={route.dir}
            canGoBack={canGoBack}
            onBack={onBack}
            accessPolicy={me.data?.access_policy}
          />
        )}
      </div>
    </div>
  );
}
