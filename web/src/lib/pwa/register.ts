// Registers the console service worker. Only a production build registers it:
// a dev server serves a new bundle on every edit, and a shell cache on top of
// that hides changes and serves stale routes.
//
// The worker itself is a hand written file at public/sw.js, outside the Next
// bundle, because a service worker cannot be bundled with the app it serves.

export const SERVICE_WORKER_URL = "/sw.js";
export const SERVICE_WORKER_SCOPE = "/";

export function serviceWorkerSupported(): boolean {
  return typeof navigator !== "undefined" && "serviceWorker" in navigator;
}

export function shouldRegister(): boolean {
  return process.env.NODE_ENV === "production" && serviceWorkerSupported();
}

/**
 * Registers the worker and returns it, or null when this build or browser
 * cannot. Never throws: a failed registration must not break the console.
 */
export async function registerServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!shouldRegister()) return null;
  try {
    const registration = await navigator.serviceWorker.register(SERVICE_WORKER_URL, {
      scope: SERVICE_WORKER_SCOPE,
      // The worker is fetched on every load, so a deploy is picked up without
      // waiting for the HTTP cache to expire.
      updateViaCache: "none",
    });
    return registration;
  } catch {
    return null;
  }
}

/** True once a worker controls this page, which is what makes offline work. */
export function isControlled(): boolean {
  return serviceWorkerSupported() && navigator.serviceWorker.controller !== null;
}
