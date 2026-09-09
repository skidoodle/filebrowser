import { Button, Input } from "@cloudflare/kumo";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeftIcon, HardDrivesIcon } from "@phosphor-icons/react";
import { useState, type SubmitEvent } from "react";
import { auth } from "../api/auth";
import { navigate, useRoute } from "../lib/router";

interface AuthScreenProps {
  mode: "setup" | "login";
}

export function AuthScreen({ mode }: AuthScreenProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const queryClient = useQueryClient();
  const route = useRoute();

  const submit = useMutation({
    mutationFn: async () => {
      if (mode === "setup") await auth.setup(password);
      else await auth.login(username, password);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      navigate({ page: "files", dir: route.dir }, { replace: true });
    },
  });

  const mismatch = mode === "setup" && confirm.length > 0 && password !== confirm;
  const tooShort = mode === "setup" && password.length > 0 && password.length < 8;
  const disabled =
    password.length === 0 ||
    (mode === "login" && username.trim().length === 0) ||
    (mode === "setup" && (confirm !== password || password.length < 8)) ||
    submit.isPending;

  const onBack = () => {
    navigate({ page: "files", dir: route.dir || "." }, { replace: true });
  };

  const onSubmit = (e: SubmitEvent) => {
    e.preventDefault();
    if (!disabled) submit.mutate();
  };

  return (
    <div className="bg-kumo-canvas flex h-full items-center justify-center p-6">
      <form onSubmit={onSubmit} className="bg-kumo-base ring-kumo-hairline w-full max-w-sm rounded-2xl p-8 shadow-sm ring-1">
        <div className="mb-6 flex items-center gap-3">
          {mode === "login" && (
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
          <h1 className="text-lg font-semibold">
            {mode === "setup" ? "Create your admin account" : "Sign-in"}
          </h1>
        </div>
        <p className="text-kumo-subtle mb-5 text-sm">
          {mode === "setup"
            ? "This server has no accounts yet. Create the admin account to unlock file management (modify, delete, move)."
            : "Sign in to unlock file management in your scope."}
        </p>
        <div className="flex flex-col gap-3">
          {mode === "login" && (
            <Input
              autoFocus
              type="text"
              value={username}
              autoComplete="username"
              placeholder="Username"
              onChange={(e) => setUsername(e.target.value)}
            />
          )}          <Input
            autoFocus={mode === "setup"}
            type="password"
            value={password}
            autoComplete={mode === "setup" ? "new-password" : "current-password"}
            placeholder={mode === "setup" ? "New password (min 8 characters)" : "Password"}
            onChange={(e) => setPassword(e.target.value)}
          />
          {mode === "setup" && (
            <Input
              type="password"
              value={confirm}
              placeholder="Repeat password"
              onChange={(e) => setConfirm(e.target.value)}
            />
          )}
          {mismatch && <p className="text-kumo-danger text-sm">Passwords do not match.</p>}
          {tooShort && <p className="text-kumo-danger text-sm">Password must be at least 8 characters.</p>}
          {submit.error instanceof Error && <p className="text-kumo-danger text-sm">{submit.error.message}</p>}
          <div className="mt-1 flex gap-2">
            {mode === "login" && (
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
              {mode === "setup" ? "Create account" : "Sign in"}
            </Button>
          </div>
        </div>
      </form>
    </div>
  );
}
