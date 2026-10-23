"use client";

import { useEffect } from "react";
import { registerServiceWorker } from "@/lib/pwa/register";

// Mounted by the console layout, on the sign in screen as well: the shell has
// to be cached before anyone signs in, otherwise an offline launch has nothing
// to show. Renders nothing.
export function ServiceWorkerRegistrar() {
  useEffect(() => {
    void registerServiceWorker();
  }, []);
  return null;
}
