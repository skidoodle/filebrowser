import { useCallback, useEffect, useRef, type RefObject } from "react";
import { isTouchPointer } from "./pointer";

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
  "border-kumo-info/70 bg-kumo-info/10 pointer-events-none select-none fixed top-0 left-0 z-[60] hidden rounded-sm border will-change-transform";

export function useMarquee(
  containerRef: RefObject<HTMLElement | null>,
  callbacks: MarqueeCallbacks,
) {
  const cbRef = useRef(callbacks);

  useEffect(() => {
    cbRef.current = callbacks;
  });

  const startMarquee = useCallback((startX: number, startY: number, isTouch: boolean) => {
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

    const onPointerMove = (ev: PointerEvent) => {
      x2 = ev.clientX;
      y2 = ev.clientY;
      if (!moved) {
        if (Math.abs(x2 - startX) < 4 && Math.abs(y2 - startY) < 4) return;
        moved = true;
        band.style.display = "block";
      }
      schedule();
    };

    const onTouchMove = (ev: TouchEvent) => {
      if (ev.touches.length !== 1) return;
      ev.preventDefault();
      const t = ev.touches[0];
      x2 = t.clientX;
      y2 = t.clientY;
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
      if (isTouch) {
        window.removeEventListener("touchmove", onTouchMove);
        window.removeEventListener("touchend", end);
        window.removeEventListener("touchcancel", end);
      } else {
        window.removeEventListener("pointermove", onPointerMove);
        window.removeEventListener("pointerup", end);
        window.removeEventListener("keydown", onKey, true);
      }
      if (raf !== 0) {
        cancelAnimationFrame(raf);
      }
      document.body.style.userSelect = "";
      band.remove();
      if (!moved && !isTouch) {
        cbRef.current.onBackgroundClick();
      }
    };

    if (isTouch) {
      window.addEventListener("touchmove", onTouchMove, { passive: false });
      window.addEventListener("touchend", end, { passive: false });
      window.addEventListener("touchcancel", end, { passive: false });
    } else {
      window.addEventListener("pointermove", onPointerMove);
      window.addEventListener("pointerup", end);
      window.addEventListener("keydown", onKey, true);
    }
  }, [containerRef]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let lastTapTime = 0;
    let lastTapX = 0;
    let lastTapY = 0;
    let startX = 0;
    let startY = 0;
    let moved = false;

    const onSingleMove = (ev: TouchEvent) => {
      if (ev.touches.length !== 1) return;
      const t = ev.touches[0];
      if (Math.abs(t.clientX - startX) > 8 || Math.abs(t.clientY - startY) > 8) {
        moved = true;
      }
    };

    const onSingleEnd = () => {
      window.removeEventListener("touchmove", onSingleMove);
      window.removeEventListener("touchend", onSingleEnd);
      if (!moved) {
        cbRef.current.onBackgroundClick();
      }
    };

    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length !== 1) return;
      const target = e.target as HTMLElement | null;
      if (target?.closest("[data-file-item], button, a, input, textarea")) return;

      const touch = e.touches[0];
      const now = Date.now();
      const timeDiff = now - lastTapTime;
      const dist = Math.hypot(touch.clientX - lastTapX, touch.clientY - lastTapY);
      const isDoubleTap = timeDiff > 40 && timeDiff < 350 && dist < 30;

      if (!isDoubleTap) {
        lastTapTime = now;
        lastTapX = touch.clientX;
        lastTapY = touch.clientY;
        startX = touch.clientX;
        startY = touch.clientY;
        moved = false;

        window.addEventListener("touchmove", onSingleMove, { passive: true });
        window.addEventListener("touchend", onSingleEnd, { passive: true });
        return;
      }

      window.removeEventListener("touchmove", onSingleMove);
      window.removeEventListener("touchend", onSingleEnd);
      e.preventDefault();
      lastTapTime = 0;
      startMarquee(touch.clientX, touch.clientY, true);
    };

    const onMouseDown = (e: MouseEvent) => {
      if (isTouchPointer(e)) return;
      if (e.button !== 0 || e.ctrlKey || e.shiftKey || e.metaKey) return;
      if ((e.target as HTMLElement).closest("[data-file-item], button, a, input, textarea")) return;
      startMarquee(e.clientX, e.clientY, false);
    };

    container.addEventListener("mousedown", onMouseDown);
    container.addEventListener("touchstart", onTouchStart, { passive: false });
    return () => {
      container.removeEventListener("mousedown", onMouseDown);
      container.removeEventListener("touchstart", onTouchStart);
      window.removeEventListener("touchmove", onSingleMove);
      window.removeEventListener("touchend", onSingleEnd);
    };
  }, [containerRef, startMarquee]);
}
