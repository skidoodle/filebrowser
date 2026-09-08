import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { auth } from "./api/auth";
import { AuthScreen } from "./components/AuthScreen";
import { NewItemDialog } from "./components/NewItemDialog";
import { SearchOverlay } from "./components/SearchOverlay";
import { Sidebar } from "./components/Sidebar";
import { UploadPanel } from "./components/UploadPanel";
import { navigate, useRoute } from "./lib/router";
import { Browser } from "./pages/Browser";
import { SettingsView } from "./pages/SettingsView";
import { usePrefs, type Theme } from "./stores/prefs";
import { ViewerOverlay } from "./viewers/ViewerOverlay";

function useThemeEffect(theme: Theme) {
  useEffect(() => {
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      const effective = theme === "system" ? (mql.matches ? "dark" : "light") : theme;
      document.documentElement.dataset.mode = effective;
    };
    apply();
    mql.addEventListener("change", apply);
    return () => mql.removeEventListener("change", apply);
  }, [theme]);
}

export default function App() {
  const prefs = usePrefs();
  const route = useRoute();
  const dir = route.dir;
  const newKind = route.page === "create" ? route.kind : null;
  const [searchOpen, setSearchOpen] = useState(false);
  const loginMode = route.page === "auth" ? route.mode : null;
  const settingsTab = route.page === "settings" ? route.tab : null;

  const me = useQuery({ queryKey: ["me"], queryFn: auth.me, staleTime: 60_000 });

  useThemeEffect(prefs.theme);

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

  if (loginMode !== null) {
    return <AuthScreen key={loginMode} mode={loginMode} />;
  }

  // The user-management tab is admin-only; everyone else gets profile.
  const effectiveTab = settingsTab === "users" && me.data && !me.data.admin ? "profile" : settingsTab;

  return (
    <div className="flex h-full overflow-hidden">
      <Sidebar currentDir={dir} onNavigate={navigateToDirectory} />

      {effectiveTab !== null && signedIn ? (
        <SettingsView
          tab={effectiveTab}
          onTabChange={(tab) => navigate({ page: "settings", tab, dir }, { replace: true })}
          onClose={() => navigate({ page: "files", dir }, { replace: true })}
        />
      ) : (
        <Browser onSearch={() => setSearchOpen(true)} />
      )}

      <UploadPanel />
      <SearchOverlay
        open={searchOpen}
        onOpenChange={setSearchOpen}
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
        onCreate={() => navigate({ page: "files", dir }, { replace: true })}
      />
      <ViewerOverlay />
    </div>
  );
}
