import { forwardRef, type MouseEvent } from "react";
import type { LinkComponentProps } from "@cloudflare/kumo";
import { navigateHref } from "../lib/router";

/** Bridges Kumo's LinkProvider to the application history router. */
export const AppLink = forwardRef<HTMLAnchorElement, LinkComponentProps>(function AppLink(
  { href, children, onClick, target, ...rest },
  ref,
) {
  const handle = (e: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(e);
    if (e.defaultPrevented) return;
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
    if (target && target !== "_self") return;
    if (!href || !navigateHref(href)) return;
    e.preventDefault();
  };

  return (
    <a ref={ref} href={href} target={target} onClick={handle} {...rest}>
      {children}
    </a>
  );
});
