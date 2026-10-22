"use client";

import type { KeyboardEvent, MouseEvent, ReactNode } from "react";

/**
 * A <details> menu that works without JavaScript. With JavaScript it also
 * closes after a link inside it is followed, and on Escape.
 */
export function Disclosure({
  summary,
  children,
  className = "",
  summaryClassName = "",
}: {
  summary: ReactNode;
  children: ReactNode;
  className?: string;
  summaryClassName?: string;
}) {
  const closeOnLink = (event: MouseEvent<HTMLDetailsElement>) => {
    if (event.target instanceof Element && event.target.closest("a")) {
      event.currentTarget.open = false;
    }
  };
  const closeOnEscape = (event: KeyboardEvent<HTMLDetailsElement>) => {
    if (event.key === "Escape" && event.currentTarget.open) {
      event.currentTarget.open = false;
      event.currentTarget.querySelector("summary")?.focus();
    }
  };
  return (
    <details className={`group ${className}`} onClick={closeOnLink} onKeyDown={closeOnEscape}>
      <summary className={`cursor-pointer list-none [&::-webkit-details-marker]:hidden ${summaryClassName}`}>
        {summary}
      </summary>
      {children}
    </details>
  );
}
