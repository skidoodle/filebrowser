import { Badge, Button, Dialog, Empty, Loader } from "@cloudflare/kumo";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowSquareOutIcon,
  CheckSquareIcon,
  DownloadSimpleIcon,
  EyeIcon,
  EyeSlashIcon,
  FilePlusIcon,
  FolderPlusIcon,
  InfoIcon,
  LinkSimpleIcon,
  PencilSimpleIcon,
  TrashSimpleIcon,
  UploadSimpleIcon,
} from "@phosphor-icons/react";
import { useEffect, useMemo, useRef, useState, type DragEvent, type MouseEvent as ReactMouseEvent } from "react";
import { api } from "../api/client";
import { auth } from "../api/auth";
import { canWriteIn, canWritePath } from "../lib/permissions";
import { ContextMenu, type ContextMenuState, type MenuEntry } from "../components/ContextMenu";
import { DirBreadcrumbs } from "../components/DirBreadcrumbs";
import { AdminDnd } from "../components/dnd";
import { FileCard } from "../components/FileCard";
import { FileTable } from "../components/FileTable";
import { InfoPanel } from "../components/InfoPanel";
import { MoveDialog } from "../components/MoveDialog";
import { TopBar } from "../components/TopBar";
import { useMarquee } from "../lib/useMarquee";
import { navigate, routePath, useRoute } from "../lib/router";
import { usePrefs } from "../stores/prefs";
import { useSelection } from "../stores/selection";
import { useUploads } from "../stores/uploads";
import type { FileInfo } from "../types";

interface BrowserProps {
  onSearch: () => void;
  onOpenMobileMenu?: () => void;
}

export function Browser({ onSearch, onOpenMobileMenu }: BrowserProps) {
  const route = useRoute();
  const dir = route.dir;
  const prefs = usePrefs();
  const selection = useSelection();
  const enqueue = useUploads((s) => s.enqueue);
  const [dragOver, setDragOver] = useState(false);
  const dragCounter = useRef(0);
  const [menu, setMenu] = useState<ContextMenuState | null>(null);
  const [moveTarget, setMoveTarget] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string[] | null>(null);
  const folderInput = useRef<HTMLInputElement>(null);
  const listingRef = useRef<HTMLDivElement>(null);
  const queryClient = useQueryClient();

  const me = useQuery({ queryKey: ["me"], queryFn: auth.me, staleTime: 60_000 });
  const canWriteHere = canWriteIn(me.data, dir);

  const list = useQuery({
    queryKey: ["list", dir, prefs.sortBy, prefs.sortOrder],
    queryFn: () => api.list(dir, prefs.sortBy, prefs.sortOrder),
  });

  const items = useMemo(() => list.data?.items ?? [], [list.data]);

  const privateToggle = useMutation({
    mutationFn: ({ path, makePrivate }: { path: string; makePrivate: boolean }) =>
      makePrivate ? auth.setPrivate(path) : auth.unsetPrivate(path),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["list"] });
    },
  });

  const del = useMutation({
    mutationFn: (paths: string[]) => api.delete(paths),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["list"] });
      void queryClient.invalidateQueries({ queryKey: ["usage"] });
      selection.clear();
      setConfirmDelete(null);
    },
  });

  const marquee = useMarquee(listingRef, {
    getCurrentSelection: () => selection.selected,
    onSelectionChange: (paths) => selection.selectAll(paths),
    onBackgroundClick: () => selection.clear(),
  });

  const dirKey = dir;
  useEffect(() => {
    selection.clear();
    // Closing the context menu on navigation is a legitimate store sync.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMenu(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dirKey]);

  const selected = selection.selected;
  const viewPath = route.page === "viewer" ? route.path : null;
  const infoSelection = viewPath ? [viewPath] : [...selected];

  // Same-dir navigation replaces (no no-op history entries); entering a
  // different directory pushes so Back walks the visited folders.
  const navigateToDirectory = (target: string) =>
    navigate({ page: "files", dir: target }, { replace: target === dir });

  const open = (file: FileInfo) => {
    if (file.isDir) {
      navigateToDirectory(file.path);
    } else {
      navigate({ page: "viewer", path: file.path, dir });
    }
  };

  /** Single click selects (ctrl/shift adds); double click opens. */
  const selectEntry = (file: FileInfo, additive: boolean) => {
    if (additive) {
      selection.toggle(file.path);
    } else {
      selection.selectAll([file.path]);
    }
  };

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    dragCounter.current = 0;
    if (!canWriteHere) return; // uploads need write access here
    if (!e.dataTransfer.types || !Array.from(e.dataTransfer.types).includes("Files")) return;
    const files = Array.from(e.dataTransfer.files);
    if (files.length > 0) enqueue(dir, files);
  };

  const selectedInfos = items.filter((i) => selected.has(i.path));
  const canDownload = selectedInfos.length > 0;
  const allSelected = items.length > 0 && selected.size === items.length;

  const downloadSelection = () => {
    if (selectedInfos.length === 0) return;
    if (selectedInfos.length === 1 && !selectedInfos[0].isDir) {
      window.location.assign(api.rawUrl(selectedInfos[0].path));
      return;
    }
    window.location.assign(api.downloadUrl(dir, selectedInfos.map((i) => i.name)));
  };

  const copyLink = (file: FileInfo) => {
    const url = file.isDir
      ? new URL(routePath({ page: "files", dir: file.path }), window.location.origin).href
      : `${window.location.origin}${api.rawUrl(file.path)}`;
    void navigator.clipboard.writeText(url);
  };

  const itemContextMenu = (file: FileInfo, e: ReactMouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!selected.has(file.path)) {
      selection.selectAll([file.path]);
    }
    const targets = selected.has(file.path) ? [...selected] : [file.path];
    const canWriteItem = canWritePath(me.data, file.path);
    const entries: (MenuEntry | null)[] = [
      { label: "Open", icon: <ArrowSquareOutIcon size={16} />, onSelect: () => open(file) },
      {
        label: file.isDir ? "Download as zip" : "Download",
        icon: <DownloadSimpleIcon size={16} />,
        onSelect: () => {
          if (file.isDir) {
            window.location.assign(api.downloadUrl(dir, [file.name]));
          } else {
            window.location.assign(api.rawUrl(file.path));
          }
        },
      },
      // Privacy toggle for directories the caller may write.
      ...(file.isDir && canWriteItem
        ? ([
          {
            label: file.private ? "Make public" : "Make private",
            icon: file.private ? <EyeIcon size={16} /> : <EyeSlashIcon size={16} />,
            onSelect: () => privateToggle.mutate({ path: file.path, makePrivate: !file.private }),
          },
        ] satisfies (MenuEntry | null)[])
        : []),
      ...(canWriteItem
        ? ([
          null,
          {
            label: "Rename / move",
            icon: <PencilSimpleIcon size={16} />,
            onSelect: () => setMoveTarget(file.path),
          },
          {
            label: targets.length > 1 ? `Delete ${targets.length} items` : "Delete",
            icon: <TrashSimpleIcon size={16} />,
            danger: true,
            onSelect: () => setConfirmDelete(targets),
          },
        ] satisfies (MenuEntry | null)[])
        : []),
      null,
      {
        label: "Properties",
        icon: <InfoIcon size={16} />,
        onSelect: () => {
          if (!prefs.infoPanel) prefs.toggleInfoPanel();
        },
      },
      { label: "Copy link", icon: <LinkSimpleIcon size={16} />, onSelect: () => copyLink(file) },
    ];
    setMenu({ x: e.clientX, y: e.clientY, entries });
  };

  const backgroundMenu = (e: ReactMouseEvent) => {
    e.preventDefault();
    setMenu({
      x: e.clientX,
      y: e.clientY,
      entries: [
        ...(canWriteHere
          ? ([
            {
              label: "New folder",
              icon: <FolderPlusIcon size={16} />,
              onSelect: () => navigate({ page: "create", kind: "dir", dir }),
            },
            {
              label: "New file",
              icon: <FilePlusIcon size={16} />,
              onSelect: () => navigate({ page: "create", kind: "file", dir }),
            },
            { label: "Upload files", icon: <UploadSimpleIcon size={16} />, onSelect: () => folderInput.current?.click() },
            null,
          ] satisfies (MenuEntry | null)[])
          : []),
        { label: "Select all", icon: <CheckSquareIcon size={16} />, onSelect: () => selection.selectAll(items.map((i) => i.path)) },
        {
          label: "Download folder",
          icon: <DownloadSimpleIcon size={16} />,
          onSelect: () => window.location.assign(api.downloadUrl(dir)),
        },
      ],
    });
  };

  return (
    <div className="flex h-full min-w-0 flex-1 flex-col">
      <TopBar
        onOpenMobileMenu={onOpenMobileMenu}
        onSearch={onSearch}
        onUploadFiles={canWriteHere ? (files) => enqueue(dir, files) : undefined}
        onDownload={downloadSelection}
        canDownload={canDownload}
        allSelected={allSelected}
        onSelectAll={() => (allSelected ? selection.clear() : selection.selectAll(items.map((i) => i.path)))}
      />

      <div className="flex min-h-0 flex-1">
        <AdminDnd
          enabled={canWriteHere}
          items={items}
          selection={selected}
          onMoved={() => {
            void queryClient.invalidateQueries({ queryKey: ["list"] });
            void queryClient.invalidateQueries({ queryKey: ["usage"] });
          }}
        >
          <main
            ref={listingRef}
            className="relative min-w-0 flex-1 overflow-y-auto scrollbar-gutter-stable p-3 select-none md:p-4"
            onDragEnter={(e) => {
              e.preventDefault();
              if (!e.dataTransfer.types || !Array.from(e.dataTransfer.types).includes("Files")) return;
              dragCounter.current++;
              setDragOver(true);
            }}
            onDragOver={(e) => e.preventDefault()}
            onDragLeave={(e) => {
              if (!e.dataTransfer.types || !Array.from(e.dataTransfer.types).includes("Files")) return;
              dragCounter.current--;
              if (dragCounter.current <= 0) {
                dragCounter.current = 0;
                setDragOver(false);
              }
            }}
            onDrop={onDrop}
            onMouseDown={marquee}
            onContextMenu={backgroundMenu}
          >
            <div className="flex items-center gap-2">
              <DirBreadcrumbs dir={dir} admin={canWriteHere} />
              {selected.size > 0 && (
                <Badge variant="secondary" className="ml-auto shrink-0">
                  {selected.size} selected
                </Badge>
              )}
            </div>

            {list.isPending && <Loader className="mx-auto mt-16" />}
            {list.isError && <p className="text-kumo-danger mt-8">{list.error.message}</p>}

            {!list.isPending && !list.isError && items.length === 0 && (
              <Empty
                title="Nothing here yet"
                description="Drag & drop files anywhere on this page to upload them."
              />
            )}

            {items.length > 0 && prefs.viewMode === "list" && (
              <FileTable
                items={items}
                selected={selected}
                onSelect={selectEntry}
                onOpen={open}
                onMenu={itemContextMenu}
              />
            )}

            {items.length > 0 && prefs.viewMode === "mosaic" && (
              <div className="grid grid-cols-[repeat(auto-fill,minmax(230px,1fr))] gap-3">
                {items.map((file) => (
                  <FileCard
                    key={file.path}
                    file={file}
                    selected={selected.has(file.path)}
                    onSelect={selectEntry}
                    onOpen={open}
                    onMenu={itemContextMenu}
                  />
                ))}
              </div>
            )}

            {items.length > 0 && prefs.viewMode === "gallery" && (
              <GalleryGrid items={items.filter((f) => f.type === "image")} selected={selected} onSelect={selectEntry} onOpen={open} onMenu={itemContextMenu} />
            )}

            {dragOver && (
              <div className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center bg-kumo-canvas/80 backdrop-blur-sm animate-[fadeIn_150ms_ease-out]">
                <div className="border-kumo-info flex flex-col items-center gap-3 rounded-2xl border-2 border-dashed px-12 py-10">
                  <UploadSimpleIcon size={40} weight="duotone" className="text-kumo-info" />
                  <p className="text-kumo-default text-base font-semibold">Drop files to upload</p>
                  <p className="text-kumo-subtle text-sm">Files will be uploaded to this folder</p>
                </div>
              </div>
            )}
          </main>
        </AdminDnd>

        {prefs.infoPanel && <InfoPanel paths={infoSelection} dir={dir} />}
      </div>

      <ContextMenu menu={menu} onClose={() => setMenu(null)} />

      {moveTarget !== null && (
        <MoveDialog key={moveTarget} from={moveTarget} onClose={() => setMoveTarget(null)} />
      )}

      <Dialog.Root
        open={confirmDelete !== null}
        onOpenChange={(open) => {
          if (!open) setConfirmDelete(null);
        }}
      >
        <Dialog className="p-6">
          <Dialog.Title>Delete</Dialog.Title>
          <Dialog.Description>
            {confirmDelete !== null && confirmDelete.length > 1
              ? `Permanently delete ${confirmDelete.length} items? This cannot be undone.`
              : "Permanently delete this item? This cannot be undone."}
          </Dialog.Description>
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setConfirmDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={del.isPending}
              onClick={() => confirmDelete !== null && del.mutate(confirmDelete)}
            >
              Delete
            </Button>
          </div>
        </Dialog>
      </Dialog.Root>

      <input
        ref={folderInput}
        type="file"
        multiple
        hidden
        // @ts-expect-error non-standard but widely supported directory picker
        webkitdirectory="true"
        directory=""
        onChange={(e) => {
          if (e.target.files) enqueue(dir, Array.from(e.target.files));
          e.target.value = "";
        }}
      />
    </div>
  );
}

interface GalleryGridProps {
  items: FileInfo[];
  selected: Set<string>;
  onSelect: (file: FileInfo, additive: boolean) => void;
  onOpen: (file: FileInfo) => void;
  onMenu: (file: FileInfo, e: ReactMouseEvent<HTMLElement>) => void;
}

function GalleryGrid({ items, selected, onSelect, onOpen, onMenu }: GalleryGridProps) {
  if (items.length === 0) {
    return (
      <Empty
        title="No images in this folder"
        description="Gallery view shows image files; try mosaic or list for everything else."
      />
    );
  }
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-3">
      {items.map((file) => (
        <button
          key={file.path}
          type="button"
          data-file-item
          data-path={file.path}
          draggable={false}
          onDragStart={(e) => e.preventDefault()}
          onClick={(e) => onSelect(file, e.ctrlKey || e.metaKey || e.shiftKey)}
          onDoubleClick={() => onOpen(file)}
          onContextMenu={(e) => onMenu(file, e)}
          className={`bg-kumo-base ring-kumo-hairline group relative overflow-hidden rounded-xl ring-1 ${selected.has(file.path) ? "ring-2 ring-kumo-info" : ""
            }`}
        >
          <img
            src={api.thumbUrl(file.path)}
            alt={file.name}
            loading="lazy"
            draggable={false}
            className="aspect-square w-full object-cover pointer-events-none select-none"
          />
          <p className="truncate px-3 py-2 text-left text-xs leading-5 select-none" title={file.name}>
            {file.name}
          </p>
        </button>
      ))}
    </div>
  );
}
