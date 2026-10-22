"use client";

import { useSyncExternalStore } from "react";

const subscribe = (): (() => void) => () => {};

/** False during server render and hydration, true afterwards. */
export function useHydrated(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => true,
    () => false,
  );
}
