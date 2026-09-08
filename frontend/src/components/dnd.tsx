import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { useMemo, useState, type ReactNode } from "react";
import { withBasePath } from "../lib/base";
import { joinPath } from "../lib/path";
import { FileTypeIcon } from "../lib/icons";
import type { FileInfo } from "../types";

export interface DragProps {
  ref: (element: HTMLElement | null) => void;
  isDragging: boolean;
  handleProps: Record<string, unknown>;
}

export interface DropProps {
  ref: (element: HTMLElement | null) => void;
  isOver: boolean;
}

interface DragPayload {
  path: string;
  isDir: boolean;
}

export function useItemDrag(file: FileInfo, enabled: boolean): DragProps {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: "drag:" + file.path,
    data: { path: file.path, isDir: file.isDir } satisfies DragPayload,
    disabled: !enabled,
  });
  return {
    ref: setNodeRef,
    isDragging,
    handleProps: enabled ? { ...attributes, ...listeners } : {},
  };
}

type DropPayload = DragPayload;

export function useFolderDrop(target: { path: string; isDir: boolean }, enabled: boolean): DropProps {
  const { setNodeRef, isOver } = useDroppable({
    id: "drop:" + target.path,
    data: { path: target.path, isDir: true } satisfies DropPayload,
    disabled: !enabled || !target.isDir,
  });
  return { ref: setNodeRef, isOver };
}

interface AdminDndProps {
  enabled: boolean;
  items: FileInfo[];
  selection: ReadonlySet<string>;
  onMoved: () => void;
  children: ReactNode;
}

export function AdminDnd({ enabled, items, selection, onMoved, children }: AdminDndProps) {
  const [source, setSource] = useState<DragPayload | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));

  const sourceInfo = useMemo(
    () => (source ? (items.find((i) => i.path === source.path) ?? null) : null),
    [source, items],
  );
  const extraCount = sourceInfo && selection.has(sourceInfo.path) ? selection.size - 1 : 0;

  const onDragStart = (event: DragStartEvent) => {
    setSource(event.active.data.current as DragPayload | null);
  };

  const onDragEnd = async (event: DragEndEvent) => {
    setSource(null);
    if (!enabled || !event.over) return;
    const source = event.active.data.current as DragPayload | undefined;
    const target = event.over.data.current as DropPayload | undefined;
    if (!source || !target?.isDir) return;
    if (source.path === target.path || target.path.startsWith(source.path + "/")) return;
    const paths = source.path !== "." && selection.has(source.path) ? [...selection] : [source.path];
    const moves = paths
      .map((from) => ({ from, to: joinPath(target.path, from.split("/").pop() ?? from) }))
      .filter((m) => m.from !== m.to);
    if (moves.length === 0) return;
    const responses = await Promise.all(
      moves.map(({ from, to }) =>
        fetch(withBasePath("/api/move"), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ from, to }),
        }),
      ),
    );
    if (responses.some((r) => !r.ok)) {
      const failed = responses.find((r) => !r.ok);
      throw new Error(failed ? ((await failed.json()) as { error?: string }).error ?? failed.statusText : "move failed");
    }
    onMoved();
  };

  return (
    <DndContext
      sensors={sensors}
      onDragStart={onDragStart}
      onDragEnd={(event) => void onDragEnd(event)}
    >
      {children}
      <DragOverlay dropAnimation={{ duration: 180, easing: "cubic-bezier(0.18, 0.67, 0.6, 1.22)" }}>
        {sourceInfo && (
          <div className="bg-kumo-elevated ring-kumo-line pointer-events-none flex items-center gap-2.5 rounded-xl px-3.5 py-2.5 shadow-xl ring-1">
            <FileTypeIcon type={sourceInfo.type} size={22} />
            <span className="text-kumo-default max-w-56 truncate text-sm font-medium">{sourceInfo.name}</span>
            {extraCount > 0 && (
              <span className="bg-kumo-info-tint text-kumo-info rounded-full px-2 py-0.5 text-xs font-semibold">
                +{extraCount}
              </span>
            )}
          </div>
        )}
      </DragOverlay>
    </DndContext>
  );
}
