import { Button } from "@cloudflare/kumo";
import {
  ArrowLeftIcon,
  InfoIcon,
  KeyIcon,
  ListIcon,
  LockKeyIcon,
  ShieldCheckIcon,
  SlidersHorizontalIcon,
} from "@phosphor-icons/react";
import { useMe } from "../lib/useMe";
import type { SettingsTab } from "../lib/routes";
import {
  AboutPanel,
  PolicyPanel,
  ProfilePanel,
  SystemPanel,
  UsersPanel,
} from "./settings";

export type { SettingsTab } from "../lib/routes";

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
