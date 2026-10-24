// 9. PWA: the manifest, the service worker in production, and the offline
// behaviour for the gate and for a route that was never visited.

import { test, Probe } from "../lib/evidence";
import { BASE_URL } from "../lib/env";
import { startKillableProxy } from "../lib/offline";

test.describe("pwa", () => {
  test("the web app manifest is served and complete", async ({ request, probe }) => {
    const response = await request.get(`${BASE_URL}/manifest.webmanifest`, { failOnStatusCode: false });
    await probe.check("the manifest answers with 200", response.status() === 200, `status ${response.status()}`);

    const body = await response.text();
    let manifest: Record<string, unknown> = {};
    try {
      manifest = JSON.parse(body) as Record<string, unknown>;
    } catch {
      await probe.fail("the manifest is valid json", body.slice(0, 200));
    }

    await probe.check("the manifest names the app", manifest.name === "multi.ai.inbox console", String(manifest.name));
    await probe.check("the manifest sets start_url to the console", manifest.start_url === "/console/", String(manifest.start_url));
    await probe.check("the manifest runs standalone", manifest.display === "standalone", String(manifest.display));
    await probe.check("the manifest sets a theme and a background colour", Boolean(manifest.theme_color) && Boolean(manifest.background_color));

    const icons = (manifest.icons ?? []) as { src: string; sizes: string; purpose?: string }[];
    await probe.check("the manifest lists at least three icons", icons.length >= 3, `${icons.length}`);
    for (const icon of icons) {
      const iconResponse = await request.get(`${BASE_URL}${icon.src}`, { failOnStatusCode: false });
      await probe.check(
        `the icon ${icon.src} is served`,
        iconResponse.status() === 200,
        `status ${iconResponse.status()}`,
      );
    }
    const maskable = icons.some((icon) => (icon.purpose ?? "").includes("maskable"));
    await probe.check("the manifest has a maskable icon", maskable);
  });

  test("the service worker registers in the production build", async ({ page, probe }) => {
    await page.goto(`${BASE_URL}/console/`, { waitUntil: "networkidle" });
    probe.reset();

    const registered = await page
      .evaluate(async () => {
        if (!("serviceWorker" in navigator)) return "no serviceWorker in navigator";
        const existing = await navigator.serviceWorker.getRegistration();
        if (existing) return existing.active ? "active" : existing.installing ? "installing" : "waiting";
        await navigator.serviceWorker.ready;
        const after = await navigator.serviceWorker.getRegistration();
        return after?.active ? "active" : "not registered";
      })
      .catch((error: unknown) => (error instanceof Error ? error.message : String(error)));

    await probe.check("a service worker registration exists after load", registered === "active" || registered === "waiting" || registered === "installing", registered);

    const controlled = await page
      .evaluate(async () => {
        if (navigator.serviceWorker.controller) return true;
        await new Promise<void>((resolve) => {
          const timer = setTimeout(resolve, 8_000);
          navigator.serviceWorker.addEventListener("controllerchange", () => {
            clearTimeout(timer);
            resolve();
          });
        });
        return navigator.serviceWorker.controller !== null;
      })
      .catch(() => false);
    await probe.check("the worker controls the console page", controlled);
    await probe.clean("service worker registration");
  });

  test("with the network blocked the gate is shown and an unvisited route falls back to the offline page", async ({
    browser,
  }) => {
    // A proxy that stops answering is the only faithful way to remove the
    // network from a service worker: the offline switch still lets the worker
    // and the http cache answer.
    const proxy = await startKillableProxy();
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      proxy: { server: proxy.server, bypass: "" },
    });
    const page = await context.newPage();
    const local = new Probe(page, test.info());

    try {
      await page.goto(`${BASE_URL}/console/`, { waitUntil: "networkidle" });
      await page.evaluate(() => navigator.serviceWorker.ready).catch(() => undefined);
      await page.waitForTimeout(2_000);

      const warm = await page.evaluate(async () => {
        const names = await caches.keys();
        const cache = await caches.open(names[0] ?? "");
        const keys = await cache.keys().then((all) => all.map((r) => new URL(r.url).pathname));
        return { controller: navigator.serviceWorker.controller?.scriptURL ?? null, keys };
      });
      await local.check("the worker is installed before the network is cut", warm.controller !== null, JSON.stringify(warm));
      await local.check(
        "the shell cache holds the console and the offline page",
        warm.keys.includes("/console/") && warm.keys.includes("/offline/"),
        warm.keys.join(", "),
      );

      await proxy.kill();

      let reloadError = "";
      await page.reload({ waitUntil: "domcontentloaded" }).catch((error: unknown) => {
        reloadError = error instanceof Error ? error.message : String(error);
      });
      await local.check("the console reopens from the shell cache with no network", reloadError === "", reloadError);

      const gateVisible = await page.locator("#api-key").isVisible().catch(() => false);
      await local.check("the offline console shows the sign in gate, not cached mail", gateVisible);

      const cachedRows = await page.locator('a[href^="/console/inbox/"]').count();
      await local.check("no inbox data is shown while offline", cachedRows === 0, `${cachedRows} inbox links`);

      let navError = "";
      await page
        .goto(`${BASE_URL}/console/audit/?qa=never-visited-${Date.now().toString(36)}`, { waitUntil: "domcontentloaded" })
        .catch((error: unknown) => {
          navError = error instanceof Error ? error.message : String(error);
        });

      if (navError !== "") {
        await local.fail("a never visited route falls back to the offline page", navError);
      } else {
        const offlineHeading = page.getByRole("heading", { level: 1, name: "The console is offline" });
        await local.check(
          "a never visited route falls back to the offline page",
          await offlineHeading.isVisible().catch(() => false),
          (await page.locator("body").innerText().catch(() => "")).slice(0, 200),
        );
        await local.check(
          "the offline page says mail is not stored on the device",
          (await page.locator("body").innerText().catch(() => "")).includes("Mail is not stored on the device"),
        );
      }

      // network errors while offline are the point of the test, not a defect
      local.reset();
    } finally {
      await context.close();
      await proxy.kill().catch(() => undefined);
    }
  });
});
