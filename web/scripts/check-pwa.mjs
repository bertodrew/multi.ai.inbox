// Fails the build when the exported site stops being installable, or when the
// service worker starts caching something it must not. Runs after `next build`
// on the static export in out/, plus two source guards that cannot be checked
// from the output alone. Same posture as check-copy.mjs: no dependencies, exit
// code 1 with every problem listed.
import { readFileSync, existsSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

// fileURLToPath decodes %20: the repo path contains a space ("4TB SSD").
const out = fileURLToPath(new URL("../out/", import.meta.url));
const web = fileURLToPath(new URL("../", import.meta.url));

const problems = [];
const fail = (msg) => problems.push(msg);

function read(rel) {
  const p = join(out, rel);
  if (!existsSync(p)) {
    fail(`${rel} is missing from the export`);
    return null;
  }
  return readFileSync(p, "utf8");
}

function source(rel) {
  const p = join(web, rel);
  if (!existsSync(p)) {
    fail(`${rel} is missing from the source`);
    return "";
  }
  return readFileSync(p, "utf8");
}

function expect(ok, msg) {
  if (!ok) fail(msg);
}

// --- the manifest ---------------------------------------------------------

const rawManifest = read("manifest.webmanifest");
let manifest = null;
if (rawManifest !== null) {
  try {
    manifest = JSON.parse(rawManifest);
  } catch {
    fail("manifest.webmanifest is not valid JSON");
  }
}

if (manifest) {
  expect(typeof manifest.name === "string" && manifest.name.length > 0, "manifest: name is empty");
  expect(
    typeof manifest.short_name === "string" && manifest.short_name.length > 0 && manifest.short_name.length <= 15,
    "manifest: short_name is missing or longer than 15 characters",
  );
  expect(
    manifest.start_url === "/console/" || manifest.start_url === "/console",
    `manifest: start_url must be /console/ or /console, found ${JSON.stringify(manifest.start_url)}`,
  );
  expect(manifest.scope === "/", `manifest: scope must be /, found ${JSON.stringify(manifest.scope)}`);
  expect(
    manifest.display === "standalone" || manifest.display === "fullscreen",
    `manifest: display must be standalone, found ${JSON.stringify(manifest.display)}`,
  );
  expect(manifest.theme_color === "#14161a", `manifest: theme_color must be #14161a, found ${JSON.stringify(manifest.theme_color)}`);
  expect(
    manifest.background_color === "#f7f6f2",
    `manifest: background_color must be #f7f6f2, found ${JSON.stringify(manifest.background_color)}`,
  );

  const icons = Array.isArray(manifest.icons) ? manifest.icons : [];
  expect(icons.length >= 2, "manifest: needs a 192 and a 512 icon");
  expect(
    icons.some((i) => i.sizes === "192x192"),
    "manifest: no 192x192 icon",
  );
  expect(
    icons.some((i) => i.sizes === "512x512" && i.purpose !== "maskable"),
    "manifest: no 512x512 icon",
  );
  expect(
    icons.some((i) => i.purpose === "maskable"),
    "manifest: no maskable icon, android draws it on any shape",
  );
  for (const icon of icons) {
    if (typeof icon.src !== "string" || !icon.src.startsWith("/")) {
      fail(`manifest: icon src must be a path on this origin, found ${JSON.stringify(icon.src)}`);
      continue;
    }
    const file = join(out, icon.src);
    if (!existsSync(file) || statSync(file).size === 0) fail(`manifest: icon ${icon.src} is missing or empty in the export`);
  }
}

// --- the service worker ---------------------------------------------------

const sw = read("sw.js");
if (sw !== null) {
  // It may only ever cache the shell. These guards are what keep an API answer
  // and a signed in page out of a cache, so they have to stay in the file.
  expect(sw.includes('request.method !== "GET"'), "sw.js: no method guard, a mutation could be cached");
  expect(sw.includes('request.headers.has("authorization")'), "sw.js: no Authorization guard");
  expect(sw.includes("url.origin !== self.location.origin"), "sw.js: no same origin guard");
  expect(sw.includes('"/v0/"'), "sw.js: /v0/ is not excluded from caching");
  expect(sw.includes('"/mcp"'), "sw.js: /mcp is not excluded from caching");
  expect(sw.includes("/console/"), "sw.js: the console shell is not precached");
  expect(sw.includes("/offline/"), "sw.js: the offline page is not precached");
  expect(sw.includes('self.addEventListener("install"'), "sw.js: no install handler");
  expect(sw.includes('self.addEventListener("activate"'), "sw.js: no activate handler");
  expect(sw.includes('self.addEventListener("fetch"'), "sw.js: no fetch handler");
  expect(sw.includes('self.addEventListener("push"'), "sw.js: no push handler");
  expect(sw.includes("showNotification"), "sw.js: push does not render a notification");

  const external = [...sw.matchAll(/https?:\/\/[^\s"'`)]+/g)].map((m) => m[0]);
  expect(external.length === 0, `sw.js: an absolute URL breaks the same origin rule: ${external.join(", ")}`);

  // A worker that cached every GET would swallow the API through a redirect.
  expect(!sw.includes("caches.match(request)"), "sw.js: use an explicit cache, not a global caches.match sweep");
  expect(!/caches\.open\([^)]*\)\.then\([^)]*\)\.then\([^)]*addAll/.test(sw), "sw.js: cache.addAll fails the whole install on one 404");
}

// --- the offline page -----------------------------------------------------

const offline = read("offline/index.html");
if (offline !== null) {
  expect(/offline/i.test(offline), "offline page: does not say the app is offline");
  for (const pretend of ["No messages", "No threads", "Your inbox is empty", "0 messages"]) {
    expect(!offline.includes(pretend), `offline page: pretends to show mail ("${pretend}")`);
  }
}

// --- what the document declares -------------------------------------------

for (const page of ["index.html", "console/index.html"]) {
  const html = read(page);
  if (html === null) continue;
  expect(
    html.includes('rel="manifest" href="/manifest.webmanifest"'),
    `${page}: does not link the manifest`,
  );
  expect(html.includes('rel="apple-touch-icon"'), `${page}: no apple-touch-icon for iOS`);
  expect(
    html.includes('name="mobile-web-app-capable"') || html.includes('name="apple-mobile-web-app-capable"'),
    `${page}: no capable meta tag, iOS would open it in a tab`,
  );

  // CSP is 'self'. Anything a browser would fetch from another origin breaks it.
  for (const [, url] of html.matchAll(/<(?:script|link|img)[^>]+(?:src|href)="([^"]+)"/g)) {
    if (/^(https?:)?\/\//.test(url) && !url.startsWith("https://aimulti.inbox")) {
      fail(`${page}: loads ${url} from another origin`);
    }
  }
}

// The gate has to be the first thing a static request renders: the console
// page is cached, so a page that rendered inbox content here would hand a
// signed out visitor a cached inbox.
const consoleHtml = read("console/index.html");
if (consoleHtml !== null) {
  expect(
    consoleHtml.includes("Opening the console"),
    "console/index.html: the shell no longer renders the sign in gate first",
  );
  expect(
    !/inbox_id"/.test(consoleHtml),
    "console/index.html: renders inbox data in the static export",
  );
}

// --- source guards --------------------------------------------------------

const session = source("src/lib/console/session.ts");
expect(session.includes("sessionStorage"), "session.ts: the key is no longer kept in session storage");
expect(
  !/localStorage\s*[.[]/.test(session),
  "session.ts: localStorage would keep the API key on an installed app",
);
expect(!/document\.cookie/.test(session), "session.ts: a cookie would bypass the gate");

const register = source("src/lib/pwa/register.ts");
expect(
  register.includes('process.env.NODE_ENV === "production"'),
  "register.ts: the service worker would be registered in development too",
);

if (problems.length) {
  console.error(`check-pwa: the installable app is broken\n  ${problems.join("\n  ")}`);
  process.exit(1);
}
console.log("check-pwa: manifest, service worker, offline page and gates are intact");
