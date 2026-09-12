let lastPointerType = "mouse";

if (typeof window !== "undefined") {
  window.addEventListener(
    "pointerdown",
    (e) => {
      lastPointerType = e.pointerType;
    },
    { capture: true, passive: true },
  );
}

/**
 * Returns true if the event was initiated by a touch interaction.
 * Used to prevent touch gestures from triggering mouse right-click context menus.
 */
export function isTouchPointer(e?: { nativeEvent?: Event }): boolean {
  if (e?.nativeEvent && "pointerType" in e.nativeEvent) {
    return (e.nativeEvent as PointerEvent).pointerType === "touch";
  }
  return lastPointerType === "touch";
}
