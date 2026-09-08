import type { ReactNode } from "react";
import { CheckIcon } from "@phosphor-icons/react";
import { DropdownMenu } from "@cloudflare/kumo";

interface MenuItemRowProps {
  icon?: ReactNode;
  label: ReactNode;
  active?: boolean;
  onClick: () => void;
}

export function MenuItemRow({ icon, label, active, onClick }: MenuItemRowProps) {
  return (
    <DropdownMenu.Item onClick={onClick}>
      <span className="flex w-full items-center gap-2.5">
        {icon && <span className="text-kumo-subtle flex w-4 justify-center">{icon}</span>}
        <span className="flex-1">{label}</span>
        {active && <CheckIcon size={14} weight="bold" className="text-kumo-brand" />}
      </span>
    </DropdownMenu.Item>
  );
}
