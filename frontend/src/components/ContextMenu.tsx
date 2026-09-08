import type { ReactNode } from "react";
import { useEffect } from "react";

export interface MenuEntry {
  label: string;
  icon?: ReactNode;
  onSelect: () => void;
  danger?: boolean;
}

export interface ContextMenuState {
  x: number;
  y: number;
  entries: (MenuEntry | null)[];
}

interface ContextMenuProps {
  menu: ContextMenuState | null;
  onClose: () => void;
}

export function ContextMenu({ menu, onClose }: ContextMenuProps) {
  useEffect(() => {
    if (!menu) return;

    const close = () => onClose();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };

    window.addEventListener("click", close);
    window.addEventListener("resize", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("keydown", onKey);
    };
  }, [menu, onClose]);

  if (!menu) return null;

  const width = 208;
  const height = menu.entries.length * 36 + 12;
  const x = Math.min(menu.x, window.innerWidth - width - 8);
  const y = Math.min(menu.y, window.innerHeight - height - 8);

  return (
    <div
      role="menu"
      className="bg-kumo-elevated ring-kumo-line fixed z-70 min-w-52 rounded-lg p-1 shadow-lg ring"
      style={{ left: Math.max(4, x), top: Math.max(4, y) }}
      onContextMenu={(e) => e.preventDefault()}
    >
      {menu.entries.map((entry, i) =>
        entry === null ? (
          <div key={`sep-${i}`} className="border-kumo-line my-1 border-t" />
        ) : (
          <button
            key={entry.label}
            type="button"
            role="menuitem"
            onClick={entry.onSelect}
            className={`hover:bg-kumo-tint flex w-full items-center gap-2.5 rounded px-3 py-2 text-left text-sm ${entry.danger ? "text-kumo-danger" : "text-kumo-default"
              }`}
          >
            {entry.icon && <span className="text-kumo-subtle flex h-4 w-4 items-center justify-center">{entry.icon}</span>}
            {entry.label}
          </button>
        ),
      )}
    </div>
  );
}
