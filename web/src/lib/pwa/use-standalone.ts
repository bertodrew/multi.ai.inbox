"use client";

import { useSyncExternalStore } from "react";

// Installed app or browser tab. Read through useSyncExternalStore so the
// prerendered HTML and the first client render agree, and so a launch from the
// home screen icon after a tab visit is picked up.
const QUERY = "(display-mode: standalone)";

function matches(): boolean {
  const nav = window.navigator as Navigator & { standalone?: boolean };
  return window.matchMedia(QUERY).matches || nav.standalone === true;
}

function subscribe(onChange: () => void): () => void {
  const media = window.matchMedia(QUERY);
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}

/** False while prerendering and on a browser tab, true in the installed app. */
export function useStandalone(): boolean {
  return useSyncExternalStore(subscribe, matches, () => false);
}
