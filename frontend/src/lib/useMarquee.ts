import { useEffect, useRef, type MouseEvent as ReactMouseEvent, type RefObject } from "react";

export interface MarqueeCallbacks {
  getCurrentSelection: () => Set<string>;
  onSelectionChange: (paths: string[]) => void;
  onBackgroundClick: () => void;
}

interface CachedRect {
  path: string;
  left: number;
  top: number;
  right: number;
  bottom: number;
}

const BAND_CLASS =
  "border-kumo-info/70 bg-kumo-info/10 pointer-events-none fixed top-0 left-0 z-[60] hidden rounded-sm border will-change-transform";

export function useMarquee(
  containerRef: RefObject<HTMLElement | null>,
  callbacks: MarqueeCallbacks,
) {
  const cbRef = useRef(callbacks);

  useEffect(() => {
    cbRef.current = callbacks;
  });

  const onMouseDown = (e: ReactMouseEvent) => {
    if (e.button !== 0 || e.ctrlKey || e.shiftKey || e.metaKey) return;
    if ((e.target as HTMLElement).closest("[data-file-item], button, a, input, textarea")) return;
    const container = containerRef.current;
    if (!container) return;
    const bounds = container.getBoundingClientRect();
    const items: CachedRect[] = [];
    container.querySelectorAll<HTMLElement>("[data-file-item]").forEach((el) => {
      const r = el.getBoundingClientRect();
      items.push({ path: el.dataset.path ?? "", left: r.left, top: r.top, right: r.right, bottom: r.bottom });
    });

    const band = document.createElement("div");
    band.className = BAND_CLASS;
    document.body.appendChild(band);

    const startX = e.clientX;
    const startY = e.clientY;
    const base = [...cbRef.current.getCurrentSelection()];
    let x2 = startX;
    let y2 = startY;
    let moved = false;
    let raf = 0;
    let lastKey = "";

    document.body.style.userSelect = "none";

    const frame = () => {
      raf = 0;
      const left = Math.max(Math.min(startX, x2), bounds.left);
      const top = Math.max(Math.min(startY, y2), bounds.top);
      const right = Math.min(Math.max(startX, x2), bounds.right);
      const bottom = Math.min(Math.max(startY, y2), bounds.bottom);
      const width = right - left;
      const height = bottom - top;
      band.style.transform = `translate(${left}px, ${top}px)`;
      band.style.width = `${width}px`;
      band.style.height = `${height}px`;

      const hits: string[] = [];
      for (const it of items) {
        if (it.left < right && it.right > left && it.top < bottom && it.bottom > top) {
          hits.push(it.path);
        }
      }
      const key = `${hits.length}:${hits.join("|")}`;
      if (key !== lastKey) {
        lastKey = key;
        cbRef.current.onSelectionChange(base.concat(hits));
      }
    };

    const schedule = () => {
      if (raf === 0) {
        raf = requestAnimationFrame(frame);
      }
    };

    const onMove = (ev: PointerEvent) => {
      x2 = ev.clientX;
      y2 = ev.clientY;
      if (!moved) {
        if (Math.abs(x2 - startX) < 4 && Math.abs(y2 - startY) < 4) return;
        moved = true;
        band.style.display = "block";
      }
      schedule();
    };

    const onKey = (ev: KeyboardEvent) => {
      if (ev.key === "Escape") {
        end();
      }
    };

    const end = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", end);
      window.removeEventListener("keydown", onKey, true);
      if (raf !== 0) {
        cancelAnimationFrame(raf);
      }
      document.body.style.userSelect = "";
      band.remove();
      if (!moved) {
        cbRef.current.onBackgroundClick();
      }
    };

    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", end);
    window.addEventListener("keydown", onKey, true);
  };

  return onMouseDown;
}
