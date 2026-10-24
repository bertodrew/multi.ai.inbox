// Service worker for the AImulti.inbox console.
//
// It caches the app shell only: the pages Next renders to HTML, the JS and CSS
// it emits under /_next/static, the icons and the manifest. API traffic is
// never stored and never answered from disk, so no response that depends on an
// Authorization header can be replayed later. Requests for another origin (the
// API host, a push service) are left alone entirely.
//
// Registered only in production builds: see src/lib/pwa/register.ts.

const VERSION = "v1";
const SHELL_CACHE = `mai-shell-${VERSION}`;
const OFFLINE_URL = "/offline/";

const SHELL = [
  "/",
  "/console/",
  OFFLINE_URL,
  "/manifest.webmanifest",
  "/icons/icon-192.png",
  "/icons/icon-512.png",
  "/icons/icon-maskable-512.png",
  "/icons/apple-touch-icon.png",
];

// The API paths, in case the console is ever served from the API origin. An
// API on another origin is already refused by the origin check below.
const API_PREFIXES = ["/v0/", "/mcp", "/openapi.yaml"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(SHELL_CACHE);
      // allSettled: one unreachable icon must not fail the whole install.
      await Promise.allSettled(SHELL.map((url) => cache.add(new Request(url, { cache: "reload" }))));
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const names = await caches.keys();
      await Promise.all(names.filter((name) => name !== SHELL_CACHE).map((name) => caches.delete(name)));
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;

  // Sends, replies and every other mutation go straight to the network.
  if (request.method !== "GET") return;
  // Anything carrying a key is never written to a cache.
  if (request.headers.has("authorization")) return;

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  if (API_PREFIXES.some((prefix) => url.pathname.startsWith(prefix))) return;

  if (request.mode === "navigate") {
    event.respondWith(navigate(request));
    return;
  }
  if (url.pathname.startsWith("/_next/static/") || url.pathname.startsWith("/icons/")) {
    event.respondWith(cacheFirst(request));
    return;
  }
  // The manifest is small and cheap to refetch; leave it to the browser.
  if (url.pathname === "/manifest.webmanifest") return;
  event.respondWith(staleWhileRevalidate(request));
});

// Network first so a deploy is picked up at once, with the cached copy of the
// same path as the fallback and the offline page after that. A page that shows
// mail is never served from cache while the network answers, so a stale inbox
// cannot be mistaken for a live one.
async function navigate(request) {
  const cache = await caches.open(SHELL_CACHE);
  const key = shellKey(request.url);
  try {
    const fresh = await fetch(request);
    if (fresh.ok && fresh.type === "basic") {
      const copy = fresh.clone();
      cache.put(key, copy).catch(() => {});
    }
    return fresh;
  } catch {
    const cached = (await cache.match(request)) || (await cache.match(key));
    if (cached) return cached;
    const offline = await cache.match(OFFLINE_URL);
    return offline || offlineResponse();
  }
}

async function cacheFirst(request) {
  const cache = await caches.open(SHELL_CACHE);
  const cached = await cache.match(request);
  if (cached) return cached;
  const fresh = await fetch(request);
  if (fresh.ok && fresh.type === "basic") {
    const copy = fresh.clone();
    cache.put(request, copy).catch(() => {});
  }
  return fresh;
}

async function staleWhileRevalidate(request) {
  const cache = await caches.open(SHELL_CACHE);
  const cached = await cache.match(request);
  const network = fetch(request)
    .then((fresh) => {
      if (fresh.ok && fresh.type === "basic") {
        const copy = fresh.clone();
        cache.put(request, copy).catch(() => {});
      }
      return fresh;
    })
    .catch(() => null);
  if (cached) return cached;
  const fresh = await network;
  return fresh || Response.error();
}

// One cache entry per path: the console reads its query string on the client,
// so /console/inbox/?address=x and /console/inbox/ share the same shell.
function shellKey(rawUrl) {
  const url = new URL(rawUrl);
  return new Request(url.origin + url.pathname);
}

// Last resort when even the offline page was never cached. No styles and no
// scripts, so it needs nothing from the CSP.
function offlineResponse() {
  return new Response(
    "<!doctype html><html lang=\"en\"><meta charset=\"utf-8\">" +
      "<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">" +
      "<title>Offline</title><h1>The console is offline</h1>" +
      "<p>This device has no connection, and no page of the console was stored yet.</p>" +
      "<p>Reconnect and load the console once to store it for next time.</p>",
    { status: 200, headers: { "Content-Type": "text/html; charset=utf-8" } },
  );
}

// Web push. The notification is rendered from the message the push service
// delivers; nothing is fetched. The payload holds the inbox address and a path
// into the console, never mail content: it is read on a lock screen. See
// web/docs/push-design.md and internal/core/push.go.
self.addEventListener("push", (event) => {
  let payload = {};
  if (event.data) {
    try {
      payload = event.data.json();
    } catch {
      payload = { body: event.data.text() };
    }
  }
  const title = typeof payload.title === "string" && payload.title ? payload.title : "New mail";
  const body = typeof payload.body === "string" ? payload.body : "Open the console to read it.";
  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      icon: "/icons/icon-192.png",
      badge: "/icons/icon-192.png",
      tag: typeof payload.tag === "string" && payload.tag ? payload.tag : "mai-mail",
      data: { url: consolePath(payload.url) },
    }),
  );
});

// A path this origin can open. Anything absolute, of another origin or not a
// path at all falls back to the console home: a payload can never navigate the
// installed app somewhere else.
function consolePath(value) {
  if (typeof value !== "string" || !value.startsWith("/")) return "/console/";
  return value;
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = new URL(event.notification.data && event.notification.data.url ? event.notification.data.url : "/console/", self.location.origin).href;
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      for (const client of windows) {
        if (client.url.startsWith(self.location.origin) && "focus" in client) {
          await client.focus();
          return;
        }
      }
      await self.clients.openWindow(target);
    })(),
  );
});
