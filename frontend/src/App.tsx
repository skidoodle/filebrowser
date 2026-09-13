import { Loader } from "@cloudflare/kumo";
import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { auth } from "./api/auth";
import { NewItemDialog } from "./components/NewItemDialog";
import { SearchOverlay } from "./components/SearchOverlay";
import { Sidebar } from "./components/Sidebar";
import { UploadPanel } from "./components/UploadPanel";
import { navigate, useRoute, type SettingsTab } from "./lib/router";
import { Browser } from "./pages/Browser";
import { ViewerOverlay } from "./viewers/ViewerOverlay";

const SettingsView = lazy(() =>
  import("./pages/SettingsView").then((m) => ({ default: m.SettingsView }))
);
const AuthScreen = lazy(() =>
  import("./components/AuthScreen").then((m) => ({ default: m.AuthScreen }))
);

function getEffectiveSettingsTab(
  settingsTab: SettingsTab | null,
  user: { admin?: boolean; insecure?: boolean } | null | undefined
): SettingsTab | null {
  if (
    (settingsTab === "users" || settingsTab === "policy" || settingsTab === "system" || settingsTab === "about") &&
    user &&
    !user.admin
  ) {
    return "profile";
  }
  if ((settingsTab === "users" || settingsTab === "policy") && user?.insecure) {
    return "profile";
  }
  return settingsTab;
}

function useAuthGuard(
  meData: ReturnType<typeof auth.me> extends Promise<infer T> ? T | undefined : undefined,
  route: ReturnType<typeof useRoute>
) {
  const dir = route.dir;
  const loginMode = route.page === "auth" ? route.mode : null;
  const settingsTab = route.page === "settings" ? route.tab : null;
  const signedIn = meData?.insecure === true || !!meData?.username;

  // Onboarding an uninitialized server redirects the first visit to the create-account screen.
  useEffect(() => {
    if (meData && !meData.insecure && !meData.initialized && loginMode === null) {
      navigate({ page: "auth", mode: "setup", dir }, { replace: true });
    }
  }, [meData, loginMode, dir]);

  // Settings require a signed-in account; anonymous visitors never see the
  // profile/users surface (the settings route is replaced with sign-in).
  useEffect(() => {
    if (meData && settingsTab !== null && !signedIn) {
      navigate({ page: "auth", mode: "login", dir }, { replace: true });
    }
  }, [meData, settingsTab, signedIn, dir]);

  // Private access policy requires sign-in for everything.
  useEffect(() => {
    if (meData && meData.access_policy === "private" && !signedIn && loginMode === null) {
      navigate({ page: "auth", mode: "login", dir }, { replace: true });
    }
  }, [meData, signedIn, loginMode, dir]);

  return { signedIn, loginMode, settingsTab };
}

export default function App() {
  const route = useRoute();
  const dir = route.dir;
  const newKind = route.page === "create" ? route.kind : null;
  const [searchOpen, setSearchOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const closeMobileMenu = useCallback(() => setMobileMenuOpen(false), []);
  const openMobileMenu = useCallback(() => setMobileMenuOpen(true), []);

  const me = useQuery({ queryKey: ["me"], queryFn: auth.me, staleTime: 60_000 });
  const { signedIn, loginMode, settingsTab } = useAuthGuard(me.data, route);

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

  if (loginMode !== null) {
    return (
      <Suspense fallback={<div className="flex h-screen w-screen items-center justify-center bg-kumo-canvas"><Loader size="lg" /></div>}>
        <AuthScreen key={loginMode} mode={loginMode} />
      </Suspense>
    );
  }

  const effectiveTab = getEffectiveSettingsTab(settingsTab, me.data);

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
