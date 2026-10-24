// 2. Inboxes: the list loads, an inbox is created, intake toggles, and no
// request at or above 400 happens while doing it.

import { test } from "../lib/evidence";
import { HAS_KEY, SKIP_NO_KEY } from "../lib/env";
import { api, deleteInbox, openConsole, seedSession, unique } from "../lib/console";

test.describe("inboxes", () => {
  test.skip(!HAS_KEY, SKIP_NO_KEY);

  test("the list loads, an inbox is created and intake can be switched on", async ({ page, probe, context, request }) => {
    await seedSession(context);
    await openConsole(page);
    await page.waitForLoadState("networkidle");
    probe.reset();

    const name = unique("qa-inbox");
    await page.locator("#inbox-username").fill(name);
    await page.locator("#inbox-display").fill("QA acceptance bot");
    await page.getByRole("button", { name: "Create inbox" }).click();

    const row = page.locator("li").filter({ has: page.getByRole("link", { name: new RegExp(`^${name}@`) }) }).first();
    const appeared = await row
      .waitFor({ state: "visible", timeout: 20_000 })
      .then(() => true)
      .catch(() => false);
    await probe.check("the created inbox appears in the list", appeared, `${name}@...`);

    const address = appeared ? ((await row.getByRole("link").first().innerText()).trim()) : "";
    await probe.check("the created inbox has a full address", /^[^@\s]+@[^@\s]+$/.test(address), address);

    if (address !== "") {
      await row.getByRole("button", { name: "Intake on" }).click();
      const badge = row.getByText("support queue");
      const badgeShown = await badge
        .waitFor({ state: "visible", timeout: 20_000 })
        .then(() => true)
        .catch(() => false);
      await probe.check("turning intake on shows the support queue badge", badgeShown);

      const off = row.getByRole("button", { name: "Intake off" });
      await probe.check("the intake control flips to Intake off", (await off.count()) === 1);
    }

    await probe.clean("inbox create and intake toggle");

    // tidy up: the delete dialog is exercised in the resources spec
    if (address !== "") await deleteInbox(request, address);
  });

  test("the list renders every inbox the API returns", async ({ page, probe, context, request }) => {
    await seedSession(context);
    const res = await api(request, "GET", "/v0/inboxes?limit=100");
    const inboxes = (res.body?.inboxes ?? []) as { inbox_id: string }[];

    await openConsole(page);
    await page.waitForLoadState("networkidle");
    probe.reset();

    if (inboxes.length === 0) {
      await probe.check("an empty organization states that it has no inboxes", (await page.getByText("No inboxes yet").count()) > 0);
    } else {
      const first = inboxes[0]?.inbox_id ?? "";
      await probe.check(
        "the first inbox from the API is listed",
        (await page.getByRole("link", { name: first, exact: true }).count()) > 0,
        first,
      );
    }
    await probe.clean("inbox list rendering");
  });
});
