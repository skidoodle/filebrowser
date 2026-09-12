import { CaretRightIcon, HouseIcon } from "@phosphor-icons/react";
import { Button, useLinkComponent } from "@cloudflare/kumo";
import { useFolderDrop } from "./dnd";
import { navigate, routePath } from "../lib/router";

interface DirBreadcrumbsProps {
  dir: string;
  admin?: boolean;
}

export function DirBreadcrumbs({ dir, admin = false }: DirBreadcrumbsProps) {
  const Link = useLinkComponent();
  const segments = dir === "." || dir === "" ? [] : dir.split("/").filter(Boolean);

  const atRoot = segments.length === 0;
  const goHome = () => navigate({ page: "files", dir: "." }, { replace: atRoot });

  return (
    <nav aria-label="Folder path" className="mb-3 flex h-9 min-w-0 items-center gap-0.5 px-1 select-none">
      <DropTarget path="" enabled={admin} className="inline-flex rounded">
        <Button
          variant="ghost"
          shape="square"
          aria-label="Go to Files"
          title="Go to Files"
          icon={<HouseIcon size={18} weight="fill" className="text-kumo-subtle" />}
          onClick={goHome}
        />
      </DropTarget>
      {segments.map((seg, i) => {
        const target = segments.slice(0, i + 1).join("/");
        const isLast = i === segments.length - 1;
        return (
          <span key={target} className="flex min-w-0 items-center gap-0.5">
            <CaretRightIcon size={14} className="text-kumo-subtle ml-0.5 shrink-0" />
            {isLast ? (
              <span className="truncate px-1.5 py-1.5 text-sm leading-6 font-semibold">{seg}</span>
            ) : (
              <DropTarget path={target} enabled={admin} className="truncate rounded">
                <Link
                  href={routePath({ page: "files", dir: target })}
                  className="text-kumo-subtle hover:text-kumo-default truncate px-1.5 py-1.5 text-sm leading-6"
                  title={seg}
                >
                  {seg}
                </Link>
              </DropTarget>
            )}
          </span>
        );
      })}
    </nav>
  );
}

function DropTarget({
  path,
  enabled,
  className,
  children,
}: {
  path: string;
  enabled: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  const drop = useFolderDrop({ path, isDir: true }, enabled);
  /* eslint-disable react-hooks/refs */
  return (
    <span
      ref={drop.ref}
      className={`${className ?? ""} ${drop.isOver ? "bg-kumo-success-tint/60 ring-1 ring-kumo-success" : ""}`}
    >
      {children}
    </span>
  );
  /* eslint-enable react-hooks/refs */
}
