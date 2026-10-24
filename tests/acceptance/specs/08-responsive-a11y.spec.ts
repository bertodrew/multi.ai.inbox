// 8. Responsive and accessibility: the whole console at 390x844, horizontal
// overflow, and axe with wcag2a and wcag2aa on every screen.

import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { test } from "../lib/evidence";
import { BASE_URL, HAS_KEY, SKIP_NO_KEY } from "../lib/env";
import { createInbox, noHorizontalOverflow, openConsole, seedSession } from "../lib/console";

const publicScreens = [
  { path: "/", name: "home" },
  { path: "/console/", name: "console gate" },
  { path: "/onboarding/", name: "onboarding" },
  { path: "/login/", name: "sign in" },
  { path: "/offline/", name: "offline page" },
];

const consoleScreens = [
  { path: "/console/", name: "inboxes" },
  { path: "/console/compose/", name: "compose" },
  { path: "/console/tickets/", name: "tickets" },
  { path: "/console/keys/", name: "keys" },
  { path: "/console/webhooks/", name: "webhooks" },
  { path: "/console/domains/", name: "domains" },
  { path: "/console/audit/", name: "audit" },
  { path: "/console/review/", name: "review" },
];

async function axeReport(page: Page, probe: import("../lib/evidence").Probe, label: string): Promise<void> {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  const serious = results.violations.filter((v) => v.impact === "critical" || v.impact === "serious");
  if (serious.length === 0) return;
  const detail = serious
    .map((v) => {
      const targets = v.nodes.slice(0, 3).map((n) => n.target.join(" ")).join(", ");
      const snippet = (v.nodes[0]?.html ?? "").replace(/\s+/g, " ").slice(0, 160);
      return `${v.id} (${v.impact}, ${v.nodes.length} nodes: ${targets}) [${snippet}]`;
    })
    .join(" | ");
  await probe.fail(`axe finds critical or serious violations on ${label}`, detail);
}

test.describe("mobile 390x844", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 3 });

  test("no public screen scrolls sideways and none has critical or serious axe violations", async ({ page, probe }) => {
    for (const screen of publicScreens) {
      await page.goto(`${BASE_URL}${screen.path}`, { waitUntil: "networkidle" });
      probe.reset();
      await probe.check(`${screen.name} renders at 390 wide`, (await page.locator("body").innerText()).trim().length > 0);
      const overflow = await noHorizontalOverflow(page);
      await probe.check(`${screen.name} does not scroll sideways at 390 wide`, overflow.ok, overflow.detail);
      await axeReport(page, probe, screen.name);
      await probe.clean(`${screen.name} at 390 wide`);
    }
  });

  test("no console screen scrolls sideways and none has critical or serious axe violations", async ({ page, probe, context, request }) => {
    test.skip(!HAS_KEY, SKIP_NO_KEY);
    await seedSession(context);
    const inbox = await createInbox(request, "qa-mobile");

    for (const screen of consoleScreens) {
      await page.goto(`${BASE_URL}${screen.path}`, { waitUntil: "networkidle" });
      probe.reset();
      await probe.check(`${screen.name} renders at 390 wide`, await page.getByRole("navigation", { name: "Console" }).isVisible());
      const overflow = await noHorizontalOverflow(page);
      await probe.check(`${screen.name} does not scroll sideways at 390 wide`, overflow.ok, overflow.detail);
      await axeReport(page, probe, screen.name);
      await probe.clean(`${screen.name} at 390 wide`);
    }

    if (inbox) {
      await page.goto(`${BASE_URL}/console/inbox/?address=${encodeURIComponent(inbox.inbox_id)}`, { waitUntil: "networkidle" });
      probe.reset();
      const overflow = await noHorizontalOverflow(page);
      await probe.check("webmail does not scroll sideways at 390 wide", overflow.ok, overflow.detail);
      await axeReport(page, probe, "webmail");
      await probe.clean("webmail at 390 wide");
    }
  });

  test("the console navigation stays usable at 390 wide", async ({ page, probe, context }) => {
    test.skip(!HAS_KEY, SKIP_NO_KEY);
    await seedSession(context);
    await page.goto(`${BASE_URL}/console/`, { waitUntil: "networkidle" });
    probe.reset();

    const nav = page.getByRole("navigation", { name: "Console" });
    const links = nav.getByRole("link");
    await probe.check("every console destination is in the mobile navigation", (await links.count()) === 8, `${await links.count()} links`);

    const tooSmall = await links.evaluateAll((nodes) =>
      nodes
        .map((n) => {
          const rect = n.getBoundingClientRect();
          return { label: (n.textContent ?? "").trim(), height: Math.round(rect.height) };
        })
        .filter((n) => n.height < 44),
    );
    await probe.check("every navigation target is at least 44 tall", tooSmall.length === 0, JSON.stringify(tooSmall));
    await probe.clean("mobile navigation");
  });
});

test.describe("desktop accessibility", () => {
  test("every public screen is clean at 1440 wide", async ({ page, probe }) => {
    for (const screen of publicScreens) {
      await page.goto(`${BASE_URL}${screen.path}`, { waitUntil: "networkidle" });
      probe.reset();
      await axeReport(page, probe, screen.name);
      await probe.clean(`${screen.name} at 1440 wide`);
    }
  });

  test("every console screen is clean at 1440 wide", async ({ page, probe, context }) => {
    test.skip(!HAS_KEY, SKIP_NO_KEY);
    await seedSession(context);
    for (const screen of consoleScreens) {
      await openConsole(page, screen.path);
      await page.waitForLoadState("networkidle");
      probe.reset();
      await axeReport(page, probe, screen.name);
      await probe.clean(`${screen.name} at 1440 wide`);
    }
  });
});
