// 1. Gate: the console without a session, then with one.

import { test, expect } from "../lib/evidence";
import { BASE_URL, HAS_KEY, SKIP_NO_KEY, SESSION_KEY } from "../lib/env";
import { openConsole, seedSession } from "../lib/console";

test.describe("gate", () => {
  test("without a session the console shows the sign in form and no data", async ({ page, probe }) => {
    const requested: string[] = [];
    page.on("request", (request) => requested.push(request.url()));

    await page.goto(`${BASE_URL}/console/`, { waitUntil: "networkidle" });

    const signIn = page.getByRole("heading", { level: 1, name: "Console" });
    await probe.check("the sign in form is shown", await signIn.isVisible());

    const keyField = page.locator("#api-key");
    await probe.check("the API key field is present", (await keyField.count()) === 1);
    await probe.check("the API key field is a password input", (await keyField.getAttribute("type")) === "password");
    await probe.check(
      "the sign in button is present",
      (await page.getByRole("button", { name: "Sign in" }).count()) === 1,
    );

    const inboxList = page.getByRole("heading", { level: 2, name: "Create an inbox" });
    await probe.check("no inbox UI is rendered without a session", (await inboxList.count()) === 0);

    const consoleNav = page.getByRole("navigation", { name: "Console" });
    await probe.check("the console navigation is hidden without a session", (await consoleNav.count()) === 0);

    const dataCalls = requested.filter((url) => /\/v0\/(inboxes|tickets|api-keys|webhooks|domains|audit-logs)/.test(url));
    await probe.check(
      "no data endpoint is called without a session",
      dataCalls.length === 0,
      dataCalls.join(" | "),
    );

    const stored = await page.evaluate((key) => window.sessionStorage.getItem(key), SESSION_KEY);
    await probe.check("nothing is written to session storage by the gate", stored === null);

    const baseSummaries = page.locator("form details summary");
    const baseCount = await baseSummaries.count();
    const baseText = baseCount > 0 ? ((await baseSummaries.first().textContent()) ?? "") : "";
    await probe.check(
      "the gate points the API at the origin that serves the console",
      baseText.includes(new URL(BASE_URL).host),
      `summaries ${baseCount}, text ${baseText}`,
    );

    await probe.clean("gate page");
  });

  test("a rejected key shows an error, keeps the gate, and stores nothing", async ({ page, probe }) => {
    await page.goto(`${BASE_URL}/console/`, { waitUntil: "networkidle" });
    probe.reset();

    await page.locator("#api-key").fill(`mai_live_${"a".repeat(32)}`);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForTimeout(4_000);

    const alert = page.getByRole("alert");
    const shown = await alert.count();
    const text = shown > 0 ? await alert.first().innerText() : "";
    await probe.check("a rejected key produces a visible error", shown > 0 && text.trim() !== "", text);
    await probe.check("the error says the key was rejected", /reject|invalid|unauthor/i.test(text), text);

    const stored = await page.evaluate((key) => window.sessionStorage.getItem(key), SESSION_KEY);
    await probe.check("a rejected key is not stored as a session", stored === null, String(stored));
    await probe.check("the sign in form stays on screen", (await page.locator("#api-key").count()) === 1);

    // a rejected key is the expected 401, not a console error
    probe.reset();
  });

  test("a stored session opens the inbox list", async ({ page, probe, context }) => {
    test.skip(!HAS_KEY, SKIP_NO_KEY);
    await seedSession(context);

    await openConsole(page);
    await page.waitForLoadState("networkidle");

    const heading = page.getByRole("heading", { level: 1, name: "Inboxes" });
    await probe.check("the inboxes page renders after a reload with a session", await heading.isVisible());

    const nav = page.getByRole("navigation", { name: "Console" });
    await probe.check("the console navigation is rendered", (await nav.count()) === 1);

    const rows = page.locator("main ul > li, ul > li").filter({ has: page.locator('a[href^="/console/inbox/"]') });
    const empty = page.getByText("No inboxes yet");
    const count = await rows.count();
    const isEmpty = (await empty.count()) > 0;
    await probe.check("the inbox list loads or states that it is empty", count > 0 || isEmpty, `rows ${count}`);

    await probe.check(
      "the sign in form is gone once a session is stored",
      (await page.locator("#api-key").count()) === 0,
    );

    await probe.clean("inbox list");
  });
});
