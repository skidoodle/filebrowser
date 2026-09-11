import { Loader } from "@cloudflare/kumo";
import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { auth } from "./api/auth";
import { NewItemDialog } from "./components/NewItemDialog";
import { SearchOverlay } from "./components/SearchOverlay";
import { Sidebar } from "./components/Sidebar";
import { UploadPanel } from "./components/UploadPanel";
import { navigate, useRoute } from "./lib/router";
import { Browser } from "./pages/Browser";
import { ViewerOverlay } from "./viewers/ViewerOverlay";

const SettingsView = lazy(() =>
  import("./pages/SettingsView").then((m) => ({ default: m.SettingsView }))
);
const AuthScreen = lazy(() =>
  import("./components/AuthScreen").then((m) => ({ default: m.AuthScreen }))
);

export default function App() {
  const route = useRoute();
  const dir = route.dir;
  const newKind = route.page === "create" ? route.kind : null;
  const [searchOpen, setSearchOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const closeMobileMenu = useCallback(() => setMobileMenuOpen(false), []);
  const openMobileMenu = useCallback(() => setMobileMenuOpen(true), []);
  const loginMode = route.page === "auth" ? route.mode : null;
  const settingsTab = route.page === "settings" ? route.tab : null;

  const me = useQuery({ queryKey: ["me"], queryFn: auth.me, staleTime: 60_000 });

  // Onboarding an uninitialized server redirects the first visit to the create-account screen.
  useEffect(() => {
    if (me.data && !me.data.insecure && !me.data.initialized && loginMode === null) {
      navigate({ page: "auth", mode: "setup", dir }, { replace: true });
    }
  }, [me.data, loginMode, dir]);

  // Ctrl+K opens search.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setSearchOpen(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Navigating to the directory already shown replaces the history entry:
  // repeated clicks must not pollute back/forward with no-op states.
  const navigateToDirectory = (target: string) =>
    navigate({ page: "files", dir: target }, { replace: target === dir });

  // Settings require a signed-in account; anonymous visitors never see the
  // profile/users surface (the settings route is replaced with sign-in).
  const signedIn = me.data?.insecure === true || !!me.data?.username;
  useEffect(() => {
    if (me.data && settingsTab !== null && !signedIn) {
      navigate({ page: "auth", mode: "login", dir }, { replace: true });
    }
  }, [me.data, settingsTab, signedIn, dir]);

  // Private access policy requires sign-in for everything.
  useEffect(() => {
    if (me.data && me.data.access_policy === "private" && !signedIn && loginMode === null) {
      navigate({ page: "auth", mode: "login", dir }, { replace: true });
    }
  }, [me.data, signedIn, loginMode, dir]);

  if (loginMode !== null) {
    return (
      <Suspense fallback={<div className="flex h-screen w-screen items-center justify-center bg-kumo-canvas"><Loader size="lg" /></div>}>
        <AuthScreen key={loginMode} mode={loginMode} />
      </Suspense>
    );
  }

  // Admin-only settings tabs fallback to profile for regular accounts.
  const effectiveTab =
    (settingsTab === "users" || settingsTab === "policy" || settingsTab === "system" || settingsTab === "about") &&
      me.data &&
      !me.data.admin
      ? "profile"
      : settingsTab;

  return (
    <div className="flex h-full overflow-hidden">
      <Sidebar
        currentDir={dir}
        onNavigate={navigateToDirectory}
        mobileOpen={mobileMenuOpen}
        onMobileClose={closeMobileMenu}
      />

      {effectiveTab !== null && signedIn ? (
        <Suspense fallback={<main className="flex min-w-0 flex-1 items-center justify-center bg-kumo-canvas"><Loader size="lg" /></main>}>
          <SettingsView
            tab={effectiveTab}
            onTabChange={(tab) => navigate({ page: "settings", tab, dir }, { replace: true })}
            onClose={() => navigate({ page: "files", dir }, { replace: true })}
            onOpenMobileMenu={openMobileMenu}
          />
        </Suspense>
      ) : (
        <Browser
          onSearch={() => setSearchOpen(true)}
          onOpenMobileMenu={openMobileMenu}
        />
      )}

      <UploadPanel />
      <SearchOverlay
        open={searchOpen}
        onOpenChange={setSearchOpen}
        currentDir={dir}
        onSelect={(file) =>
          file.isDir
            ? navigateToDirectory(file.path)
            : navigate({ page: "viewer", path: file.path, dir })
        }
      />
      <NewItemDialog
        kind={newKind}
        dir={dir}
        onClose={() => navigate({ page: "files", dir }, { replace: true })}
        onCreate={(createdPath, createdKind) => {
          if (createdKind === "file") {
            navigate({ page: "viewer", path: createdPath, dir, edit: true }, { replace: true });
          } else {
            navigate({ page: "files", dir }, { replace: true });
          }
        }}
      />
      <ViewerOverlay />
    </div>
  );
}
