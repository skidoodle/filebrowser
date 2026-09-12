import { CaretRightIcon, DotsThreeIcon, FolderIcon, HouseIcon } from "@phosphor-icons/react";
import { Button, DropdownMenu } from "@cloudflare/kumo";
import { useMemo } from "react";
import { AppLink } from "./AppLink";
import { useFolderDrop } from "./dnd";
import { navigate, routePath } from "../lib/router";

interface DirBreadcrumbsProps {
  dir: string;
  admin?: boolean;
}

interface CrumbItem {
  name: string;
  path: string;
}

const COLLAPSE_THRESHOLD = 4;

export function DirBreadcrumbs({ dir, admin = false }: DirBreadcrumbsProps) {
  const segments = useMemo(
    () => (dir === "." || dir === "" ? [] : dir.split("/").filter(Boolean)),
    [dir],
  );

  const atRoot = segments.length === 0;
  const goHome = () => navigate({ page: "files", dir: "." }, { replace: atRoot });

  const shouldCollapse = segments.length > COLLAPSE_THRESHOLD;

  const firstItem: CrumbItem | null = useMemo(
    () => (shouldCollapse ? { name: segments[0], path: segments[0] } : null),
    [shouldCollapse, segments],
  );

  const hiddenItems: CrumbItem[] = useMemo(
    () =>
      shouldCollapse
        ? segments.slice(1, -2).map((seg, i) => ({
          name: seg,
          path: segments.slice(0, i + 2).join("/"),
        }))
        : [],
    [shouldCollapse, segments],
  );

  const tailItems: CrumbItem[] = useMemo(
    () =>
      shouldCollapse
        ? segments.slice(-2).map((seg, i) => ({
          name: seg,
          path: segments.slice(0, segments.length - 2 + i + 1).join("/"),
        }))
        : segments.map((seg, i) => ({
          name: seg,
          path: segments.slice(0, i + 1).join("/"),
        })),
    [shouldCollapse, segments],
  );

  return (
    <nav
      aria-label="Folder path"
      className="mb-3 flex h-9 flex-1 min-w-0 items-center gap-0.5 px-1 select-none overflow-hidden"
    >
      <DropTarget path="" enabled={admin} className="inline-flex shrink-0 rounded">
        <Button
          variant="ghost"
          shape="square"
          aria-label="Go to Files"
          title="Go to Files"
          icon={<HouseIcon size={18} weight="fill" className="text-kumo-subtle shrink-0" />}
          onClick={goHome}
          className="shrink-0"
        />
      </DropTarget>

      {shouldCollapse && firstItem && (
        <span className="hidden sm:inline-flex shrink min-w-0 items-center gap-0.5">
          <CaretRightIcon size={14} className="text-kumo-subtle ml-0.5 shrink-0" />
          <DropTarget path={firstItem.path} enabled={admin} className="truncate rounded">
            <AppLink
              href={routePath({ page: "files", dir: firstItem.path })}
              className="text-kumo-subtle hover:text-kumo-default max-w-32 md:max-w-44 truncate px-1.5 py-1 text-sm leading-6 rounded hover:bg-kumo-tint transition-colors block"
              title={firstItem.name}
            >
              {firstItem.name}
            </AppLink>
          </DropTarget>
        </span>
      )}

      {shouldCollapse && hiddenItems.length > 0 && (
        <span className="inline-flex items-center gap-0.5 shrink-0">
          <CaretRightIcon size={14} className="text-kumo-subtle ml-0.5 shrink-0" />
          <DropdownMenu>
            <DropdownMenu.Trigger
              render={(p) => (
                <button
                  {...p}
                  type="button"
                  aria-label="Show intermediate folders"
                  title={`Show ${hiddenItems.length} intermediate folders`}
                  className="text-kumo-subtle hover:text-kumo-default hover:bg-kumo-tint inline-flex h-7 items-center justify-center rounded-md px-1.5 transition-colors cursor-pointer shrink-0 border border-kumo-hairline/80 bg-kumo-base"
                >
                  <DotsThreeIcon size={16} weight="bold" className="shrink-0" />
                </button>
              )}
            />
            <DropdownMenu.Content
              align="start"
              className="min-w-48 max-w-xs max-h-72 overflow-y-auto z-60 p-1 shadow-lg border border-kumo-hairline rounded-xl bg-kumo-base"
            >
              {hiddenItems.map((item) => (
                <DropdownMenu.Item
                  key={item.path}
                  onClick={() => navigate({ page: "files", dir: item.path })}
                  className="cursor-pointer"
                >
                  <DropTarget
                    path={item.path}
                    enabled={admin}
                    className="flex w-full items-center gap-2 py-0.5 rounded min-w-0"
                  >
                    <FolderIcon size={16} weight="fill" className="text-blue-500 shrink-0" />
                    <span className="truncate text-sm font-medium" title={item.name}>
                      {item.name}
                    </span>
                  </DropTarget>
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Content>
          </DropdownMenu>
        </span>
      )}

      {tailItems.map((item, idx) => {
        const isLast = idx === tailItems.length - 1;
        const isParentWhenCollapsed = shouldCollapse && idx === 0;

        return (
          <span
            key={item.path}
            className={`items-center gap-0.5 ${isLast
              ? "flex flex-1 min-w-0"
              : isParentWhenCollapsed
                ? "hidden sm:inline-flex shrink min-w-0"
                : "flex shrink min-w-0"
              }`}
          >
            <CaretRightIcon size={14} className="text-kumo-subtle ml-0.5 shrink-0" />
            {isLast ? (
              <span
                className="truncate px-1.5 py-1 text-sm leading-6 font-semibold text-kumo-default min-w-0 flex-1"
                title={item.name}
              >
                {item.name}
              </span>
            ) : (
              <DropTarget path={item.path} enabled={admin} className="truncate rounded">
                <AppLink
                  href={routePath({ page: "files", dir: item.path })}
                  className="text-kumo-subtle hover:text-kumo-default max-w-32 sm:max-w-48 truncate px-1.5 py-1 text-sm leading-6 rounded hover:bg-kumo-tint transition-colors block"
                  title={item.name}
                >
                  {item.name}
                </AppLink>
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
