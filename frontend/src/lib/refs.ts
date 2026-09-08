/** Combines two element callbacks into one (items are drag AND drop
 * targets; both dnd-kit callbacks must fire on the same element). */
export function mergeRefs(
  a: (element: HTMLElement | null) => void,
  b: (element: HTMLElement | null) => void,
): (element: HTMLElement | null) => void {
  return (element) => {
    a(element);
    b(element);
  };
}
