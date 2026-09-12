import { useRef, useCallback, type TouchEvent, type MouseEvent as ReactMouseEvent } from "react";

interface LongPressOptions {
  onLongPress: (coords: { clientX: number; clientY: number }) => void;
  threshold?: number;
}

/**
 * useLongPress triggers onLongPress after holding touch for `threshold` ms
 * without significant movement (>10px). Cancels on move (scroll) or early release.
 * Suppresses the subsequent onClick event so the item is not accidentally clicked or opened.
 */
export function useLongPress({ onLongPress, threshold = 400 }: LongPressOptions) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const startPos = useRef<{ x: number; y: number } | null>(null);
  const isTriggered = useRef(false);

  const onTouchStart = useCallback(
    (e: TouchEvent) => {
      if (e.touches.length !== 1) return;
      const touch = e.touches[0];
      startPos.current = { x: touch.clientX, y: touch.clientY };
      isTriggered.current = false;

      timerRef.current = setTimeout(() => {
        isTriggered.current = true;
        if (typeof navigator !== "undefined" && "vibrate" in navigator) {
          try {
            navigator.vibrate(35);
          } catch {
            // vibration not supported or permissions denied
          }
        }
        onLongPress({ clientX: touch.clientX, clientY: touch.clientY });
      }, threshold);
    },
    [onLongPress, threshold],
  );

  const onTouchMove = useCallback((e: TouchEvent) => {
    if (!startPos.current || e.touches.length !== 1) return;
    const touch = e.touches[0];
    const dx = Math.abs(touch.clientX - startPos.current.x);
    const dy = Math.abs(touch.clientY - startPos.current.y);
    if (dx > 10 || dy > 10) {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      startPos.current = null;
    }
  }, []);

  const onTouchEnd = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    startPos.current = null;
  }, []);

  const onClickCapture = useCallback((e: ReactMouseEvent) => {
    if (isTriggered.current) {
      e.preventDefault();
      e.stopPropagation();
      isTriggered.current = false;
    }
  }, []);

  return {
    handlers: {
      onTouchStart,
      onTouchMove,
      onTouchEnd,
      onTouchCancel: onTouchEnd,
      onClickCapture,
    },
  };
}
