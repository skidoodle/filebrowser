import { Badge, Empty, Loader } from "@cloudflare/kumo";
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
import { DeleteDialog } from "../components/DeleteDialog";
import { DirBreadcrumbs } from "../components/DirBreadcrumbs";
import { AdminDnd } from "../components/dnd";
import { DropUploadOverlay } from "../components/DropUploadOverlay";
import { FileCard } from "../components/FileCard";
import { FileTable } from "../components/FileTable";
import { GalleryGrid } from "../components/GalleryGrid";
import { InfoPanel } from "../components/InfoPanel";
import { MoveDialog } from "../components/MoveDialog";
import { TopBar } from "../components/TopBar";
import { useMarquee } from "../lib/useMarquee";
import { navigate, routePath, useRoute } from "../lib/router";
import { usePrefs } from "../stores/prefs";
import { useSelection } from "../stores/selection";
import { useUploads } from "../stores/uploads";
import { isTouchPointer } from "../lib/pointer";
import type { FileInfo } from "../types";

function useBrowserMutations({
  dir,
  sortBy,
  sortOrder,
  onDeleteSuccess,
}: {
  dir: string;
  sortBy: any;
  sortOrder: any;
  onDeleteSuccess: () => void;
}) {
  const queryClient = useQueryClient();

  const privateToggle = useMutation({
    mutationFn: ({ path, makePrivate }: { path: string; makePrivate: boolean }) =>
      makePrivate ? auth.setPrivate(path) : auth.unsetPrivate(path),
    onMutate: async ({ path, makePrivate }) => {
      const targetQueryKey = ["list", dir, sortBy, sortOrder];
      await queryClient.cancelQueries({ queryKey: targetQueryKey });
      const previousList = queryClient.getQueryData<{ path: string; items: FileInfo[] }>(targetQueryKey);
      if (previousList) {
        queryClient.setQueryData(targetQueryKey, {
          ...previousList,
          items: previousList.items.map((item) =>
            item.path === path ? { ...item, private: makePrivate } : item
          ),
        });
      }
      return { previousList, targetQueryKey };
    },
    onError: (_err, _vars, context) => {
      if (context?.previousList) {
        queryClient.setQueryData(context.targetQueryKey, context.previousList);
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ["list"] });
    },
  });

  const del = useMutation({
    mutationFn: (paths: string[]) => api.delete(paths),
    onMutate: async (paths: string[]) => {
      const targetQueryKey = ["list", dir, sortBy, sortOrder];
      await queryClient.cancelQueries({ queryKey: targetQueryKey });
      const previousList = queryClient.getQueryData<{ path: string; items: FileInfo[] }>(targetQueryKey);
      if (previousList) {
        const pathSet = new Set(paths);
        queryClient.setQueryData(targetQueryKey, {
          ...previousList,
          items: previousList.items.filter((item) => !pathSet.has(item.path)),
        });
      }
      onDeleteSuccess();
      return { previousList, targetQueryKey };
    },
    onError: (_err, _paths, context) => {
      if (context?.previousList) {
        queryClient.setQueryData(context.targetQueryKey, context.previousList);
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ["list"] });
      void queryClient.invalidateQueries({ queryKey: ["usage"] });
    },
  });

  return { privateToggle, del };
}

function buildItemContextMenuEntries({
  file,
  targets,
  canWriteItem,
  dir,
  onOpen,
  onTogglePrivate,
  onMove,
  onDelete,
  onProperties,
  onCopyLink,
}: {
  file: FileInfo;
  targets: string[];
  canWriteItem: boolean;
  dir: string;
  onOpen: () => void;
  onTogglePrivate: () => void;
  onMove: () => void;
  onDelete: () => void;
  onProperties: () => void;
  onCopyLink: () => void;
}): (MenuEntry | null)[] {
  return [
    { label: "Open", icon: <ArrowSquareOutIcon size={16} />, onSelect: onOpen },
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
    ...(file.isDir && canWriteItem
      ? ([
        {
          label: file.private ? "Make public" : "Make private",
          icon: file.private ? <EyeIcon size={16} /> : <EyeSlashIcon size={16} />,
          onSelect: onTogglePrivate,
        },
      ] satisfies (MenuEntry | null)[])
      : []),
    ...(canWriteItem
      ? ([
        null,
        {
          label: "Rename / move",
          icon: <PencilSimpleIcon size={16} />,
          onSelect: onMove,
        },
        {
          label: targets.length > 1 ? `Delete ${targets.length} items` : "Delete",
          icon: <TrashSimpleIcon size={16} />,
          danger: true,
          onSelect: onDelete,
        },
      ] satisfies (MenuEntry | null)[])
      : []),
    null,
    {
      label: "Properties",
      icon: <InfoIcon size={16} />,
      onSelect: onProperties,
    },
    { label: "Copy link", icon: <LinkSimpleIcon size={16} />, onSelect: onCopyLink },
  ];
}

function buildBackgroundContextMenuEntries({
  canWriteHere,
  dir,
  onUploadClick,
  onSelectAll,
}: {
  canWriteHere: boolean;
  dir: string;
  onUploadClick: () => void;
  onSelectAll: () => void;
}): (MenuEntry | null)[] {
  return [
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
        { label: "Upload files", icon: <UploadSimpleIcon size={16} />, onSelect: onUploadClick },
        null,
      ] satisfies (MenuEntry | null)[])
      : []),
    { label: "Select all", icon: <CheckSquareIcon size={16} />, onSelect: onSelectAll },
    {
      label: "Download folder",
      icon: <DownloadSimpleIcon size={16} />,
      onSelect: () => window.location.assign(api.downloadUrl(dir)),
    },
  ];
}

function useFileDrop(canWriteHere: boolean, dir: string, onUpload: (dir: string, files: File[]) => void) {
  const [dragOver, setDragOver] = useState(false);
  const dragCounter = useRef(0);

  const onDragEnter = (e: DragEvent) => {
    e.preventDefault();
    if (!e.dataTransfer.types || !Array.from(e.dataTransfer.types).includes("Files")) return;
    dragCounter.current++;
    setDragOver(true);
  };

  const onDragOver = (e: DragEvent) => e.preventDefault();

  const onDragLeave = () => {
    dragCounter.current--;
    if (dragCounter.current <= 0) {
      dragCounter.current = 0;
      setDragOver(false);
    }
  };

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    dragCounter.current = 0;
    if (!canWriteHere) return;
    if (!e.dataTransfer.types || !Array.from(e.dataTransfer.types).includes("Files")) return;
    const files = Array.from(e.dataTransfer.files);
    if (files.length > 0) onUpload(dir, files);
  };

  return { dragOver, onDragEnter, onDragOver, onDragLeave, onDrop };
}

function downloadFiles(dir: string, selectedInfos: FileInfo[]) {
  if (selectedInfos.length === 0) return;
  if (selectedInfos.length === 1 && !selectedInfos[0].isDir) {
    window.location.assign(api.rawUrl(selectedInfos[0].path));
    return;
  }
  window.location.assign(api.downloadUrl(dir, selectedInfos.map((i) => i.name)));
}

function copyFileLink(file: FileInfo) {
  const url = file.isDir
    ? new URL(routePath({ page: "files", dir: file.path }), window.location.origin).href
    : `${window.location.origin}${api.rawUrl(file.path)}`;
  void navigator.clipboard.writeText(url);
}

function BrowserListingContent({
  isPending,
  isError,
  errorMessage,
  items,
  viewMode,
  selected,
  selectEntry,
  open,
  itemContextMenu,
  listingRef,
}: {
  isPending: boolean;
  isError: boolean;
  errorMessage?: string;
  items: FileInfo[];
  viewMode: "list" | "mosaic" | "gallery";
  selected: Set<string>;
  selectEntry: (file: FileInfo, additive: boolean) => void;
  open: (file: FileInfo) => void;
  itemContextMenu: (
    file: FileInfo,
    e: ReactMouseEvent | { clientX: number; clientY: number; preventDefault?: () => void; stopPropagation?: () => void },
  ) => void;
  listingRef: React.RefObject<HTMLDivElement | null>;
}) {
  if (isPending) return <Loader className="mx-auto mt-16" />;
  if (isError) return <p className="text-kumo-danger mt-8">{errorMessage}</p>;
  if (items.length === 0) {
    return (
      <Empty
        title="Nothing here yet"
        description="Drag & drop files anywhere on this page to upload them."
      />
    );
  }

  if (viewMode === "list") {
    return (
      <FileTable
        items={items}
        selected={selected}
        onSelect={selectEntry}
        onOpen={open}
        onMenu={itemContextMenu}
        scrollRef={listingRef}
      />
    );
  }

  if (viewMode === "mosaic") {
    return (
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
    );
  }

  return (
    <GalleryGrid
      items={items.filter((f) => f.type === "image")}
      selected={selected}
      onSelect={selectEntry}
      onOpen={open}
      onMenu={itemContextMenu}
    />
  );
}

function BrowserDialogs({
  menu,
  onCloseMenu,
  moveTarget,
  onCloseMove,
  confirmDelete,
  deleteLoading,
  onCloseDelete,
  onConfirmDelete,
}: {
  menu: ContextMenuState | null;
  onCloseMenu: () => void;
  moveTarget: string | null;
  onCloseMove: () => void;
  confirmDelete: string[] | null;
  deleteLoading: boolean;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
}) {
  return (
    <>
      <ContextMenu menu={menu} onClose={onCloseMenu} />
      {moveTarget !== null && (
        <MoveDialog key={moveTarget} from={moveTarget} onClose={onCloseMove} />
      )}
      <DeleteDialog
        paths={confirmDelete}
        loading={deleteLoading}
        onClose={onCloseDelete}
        onConfirm={onConfirmDelete}
      />
    </>
  );
}

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

  const { privateToggle, del } = useBrowserMutations({
    dir,
    sortBy: prefs.sortBy,
    sortOrder: prefs.sortOrder,
    onDeleteSuccess: () => {
      selection.clear();
      setConfirmDelete(null);
    },
  });

  useMarquee(listingRef, {
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

  const { dragOver, onDragEnter, onDragOver, onDragLeave, onDrop } = useFileDrop(
    canWriteHere,
    dir,
    enqueue,
  );

  const selectedInfos = items.filter((i) => selected.has(i.path));
  const canDownload = selectedInfos.length > 0;
  const allSelected = items.length > 0 && selected.size === items.length;

  const downloadSelection = () => downloadFiles(dir, selectedInfos);
  const copyLink = (file: FileInfo) => copyFileLink(file);

  const itemContextMenu = (
    file: FileInfo,
    e: ReactMouseEvent | { clientX: number; clientY: number; preventDefault?: () => void; stopPropagation?: () => void },
  ) => {
    e.preventDefault?.();
    e.stopPropagation?.();
    if (!selected.has(file.path)) {
      selection.selectAll([file.path]);
    }
    const targets = selected.has(file.path) ? [...selected] : [file.path];
    const canWriteItem = canWritePath(me.data, file.path);
    const entries = buildItemContextMenuEntries({
      file,
      targets,
      canWriteItem,
      dir,
      onOpen: () => open(file),
      onTogglePrivate: () => privateToggle.mutate({ path: file.path, makePrivate: !file.private }),
      onMove: () => setMoveTarget(file.path),
      onDelete: () => setConfirmDelete(targets),
      onProperties: () => {
        if (!prefs.infoPanel) prefs.toggleInfoPanel();
      },
      onCopyLink: () => copyLink(file),
    });
    setMenu({ x: e.clientX, y: e.clientY, entries });
  };

  const backgroundMenu = (e: ReactMouseEvent) => {
    e.preventDefault();
    if (isTouchPointer(e)) return;
    setMenu({
      x: e.clientX,
      y: e.clientY,
      entries: buildBackgroundContextMenuEntries({
        canWriteHere,
        dir,
        onUploadClick: () => folderInput.current?.click(),
        onSelectAll: () => selection.selectAll(items.map((i) => i.path)),
      }),
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
          viewMode={prefs.viewMode}
          onMoved={() => {
            void queryClient.invalidateQueries({ queryKey: ["list"] });
            void queryClient.invalidateQueries({ queryKey: ["usage"] });
          }}
        >
          <main
            ref={listingRef}
            className="relative min-w-0 flex-1 overflow-y-auto scrollbar-gutter-stable p-3 select-none touch-manipulation overscroll-contain md:p-4"
            onDragEnter={onDragEnter}
            onDragOver={onDragOver}
            onDragLeave={onDragLeave}
            onDrop={onDrop}
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

            <BrowserListingContent
              isPending={list.isPending}
              isError={list.isError}
              errorMessage={list.error?.message}
              items={items}
              viewMode={prefs.viewMode}
              selected={selected}
              selectEntry={selectEntry}
              open={open}
              itemContextMenu={itemContextMenu}
              listingRef={listingRef}
            />

            <DropUploadOverlay visible={dragOver} />
          </main>
        </AdminDnd>

        {prefs.infoPanel && <InfoPanel paths={infoSelection} dir={dir} />}
      </div>

      <BrowserDialogs
        menu={menu}
        onCloseMenu={() => setMenu(null)}
        moveTarget={moveTarget}
        onCloseMove={() => setMoveTarget(null)}
        confirmDelete={confirmDelete}
        deleteLoading={del.isPending}
        onCloseDelete={() => setConfirmDelete(null)}
        onConfirmDelete={() => confirmDelete !== null && del.mutate(confirmDelete)}
      />

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
