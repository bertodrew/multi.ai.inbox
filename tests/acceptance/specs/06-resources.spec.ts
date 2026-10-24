// 6. Keys, webhooks, domains, audit and review: every screen loads without a
// console error, and each destructive control asks for confirmation first.

import { test } from "../lib/evidence";
import { HAS_KEY, SKIP_NO_KEY } from "../lib/env";
import { createInbox, deleteInbox, openConsole, seedSession, unique } from "../lib/console";

const screens = [
  { path: "/console/keys/", heading: "API keys" },
  { path: "/console/webhooks/", heading: "Webhooks" },
  { path: "/console/domains/", heading: "Domains" },
  { path: "/console/audit/", heading: "Audit log" },
  { path: "/console/review/", heading: "Review queue" },
];

test.describe("console screens", () => {
  test.skip(!HAS_KEY, SKIP_NO_KEY);

  test("every management screen loads without a console error or a failed request", async ({ page, probe, context }) => {
    await seedSession(context);

    for (const screen of screens) {
      await openConsole(page, screen.path);
      await page.waitForLoadState("networkidle");
      probe.reset();

      await probe.check(`${screen.heading} renders its heading`, await page.getByRole("heading", { level: 1, name: screen.heading }).isVisible());
      await probe.check(
        `${screen.heading} shows content rather than a permanent loading state`,
        (await page.getByText(/Loading (API keys|webhooks|domains|the audit log|the review queue)/).count()) === 0,
      );
      await probe.clean(screen.heading);
    }
  });

  test("revoking a key asks for confirmation and cancel keeps the key", async ({ page, probe, context }) => {
    await seedSession(context);
    await openConsole(page, "/console/keys/");
    await page.waitForLoadState("networkidle");
    probe.reset();

    const name = unique("qa key");
    await page.locator("#key-name").fill(name);
    const selectAll = page.getByRole("button", { name: "Select all this key holds" });
    if ((await selectAll.count()) > 0) await selectAll.click();
    await page.getByRole("button", { name: "Create key" }).click();
    await page.waitForTimeout(3_000);

    const row = page.locator("li").filter({ hasText: name }).first();
    const created = await row.waitFor({ state: "visible", timeout: 20_000 }).then(() => true).catch(() => false);
    await probe.check("a scoped key can be created from the console", created, name);
    if (!created) {
      await probe.clean("key creation");
      return;
    }

    await row.getByRole("button", { name: "Revoke" }).click();
    const dialog = page.locator("dialog[open]");
    const opened = await dialog.waitFor({ state: "visible", timeout: 10_000 }).then(() => true).catch(() => false);
    await probe.check("revoking a key opens a confirmation dialog", opened);
    if (opened) {
      await probe.check("the dialog names the key", (await dialog.innerText()).includes(name), (await dialog.innerText()).slice(0, 200));
      await probe.check("the dialog offers a cancel button", (await dialog.getByRole("button", { name: "Cancel" }).count()) === 1);
      await dialog.getByRole("button", { name: "Cancel" }).click();
      await page.waitForTimeout(1_000);
      await probe.check("cancelling closes the dialog", (await page.locator("dialog[open]").count()) === 0);
      await probe.check("cancelling keeps the key", (await page.locator("li").filter({ hasText: name }).count()) > 0);
    }

    await probe.clean("key revocation dialog");

    // clean up the throwaway key
    await row.getByRole("button", { name: "Revoke" }).click();
    const confirm = page.locator("dialog[open]").getByRole("button", { name: "Revoke key" });
    if ((await confirm.count()) > 0) await confirm.click();
    await page.waitForTimeout(2_000);
    await probe.check("confirming the dialog revokes the key", (await page.locator("li").filter({ hasText: name }).count()) === 0);
    probe.reset();
  });

  test("deleting a webhook asks for confirmation and cancel keeps it", async ({ page, probe, context }) => {
    await seedSession(context);
    await openConsole(page, "/console/webhooks/");
    await page.waitForLoadState("networkidle");
    probe.reset();

    const url = `https://example.com/qa-hook-${Date.now().toString(36)}`;
    await page.locator("#hook-url").fill(url);
    const firstEvent = page.locator('fieldset input[type="checkbox"]').first();
    if ((await firstEvent.count()) > 0) await firstEvent.check();
    await page.getByRole("button", { name: "Create webhook" }).click();
    await page.waitForTimeout(3_000);

    const row = page.locator("li").filter({ hasText: url }).first();
    const created = await row.waitFor({ state: "visible", timeout: 20_000 }).then(() => true).catch(() => false);
    await probe.check("a webhook can be created from the console", created, url);
    if (!created) {
      await probe.clean("webhook creation");
      return;
    }

    await row.getByRole("button", { name: "Delete" }).click();
    const dialog = page.locator("dialog[open]");
    const opened = await dialog.waitFor({ state: "visible", timeout: 10_000 }).then(() => true).catch(() => false);
    await probe.check("deleting a webhook opens a confirmation dialog", opened);
    if (opened) {
      await dialog.getByRole("button", { name: "Cancel" }).click();
      await page.waitForTimeout(1_000);
      await probe.check("cancelling keeps the webhook", (await page.locator("li").filter({ hasText: url }).count()) > 0);
    }

    await probe.clean("webhook delete dialog");

    await row.getByRole("button", { name: "Delete" }).click();
    const confirm = page.locator("dialog[open]").getByRole("button", { name: "Delete webhook" });
    if ((await confirm.count()) > 0) await confirm.click();
    await page.waitForTimeout(2_000);
    await probe.check("confirming removes the webhook", (await page.locator("li").filter({ hasText: url }).count()) === 0);
    probe.reset();
  });

  test("deleting an inbox asks for confirmation and cancel keeps it", async ({ page, probe, context, request }) => {
    const inbox = await createInbox(request, "qa-confirm");
    if (!inbox) {
      test.skip(true, "the API refused to create the inbox this spec needs");
      return;
    }

    await seedSession(context);
    await openConsole(page, "/console/");
    await page.waitForLoadState("networkidle");
    probe.reset();

    const row = page.locator("li").filter({ hasText: inbox.inbox_id }).first();
    const listed = await row.waitFor({ state: "visible", timeout: 20_000 }).then(() => true).catch(() => false);
    await probe.check("the inbox is listed before deleting", listed, inbox.inbox_id);

    if (listed) {
      await row.getByRole("button", { name: "Delete" }).click();
      const dialog = page.locator("dialog[open]");
      const opened = await dialog.waitFor({ state: "visible", timeout: 10_000 }).then(() => true).catch(() => false);
      await probe.check("deleting an inbox opens a confirmation dialog", opened);
      if (opened) {
        await probe.check("the dialog names the inbox", (await dialog.innerText()).includes(inbox.inbox_id));
        await dialog.getByRole("button", { name: "Cancel" }).click();
        await page.waitForTimeout(1_000);
        await probe.check("cancelling keeps the inbox", (await page.locator("li").filter({ hasText: inbox.inbox_id }).count()) > 0);
      }
      await probe.clean("inbox delete dialog");
    }

    await deleteInbox(request, inbox.inbox_id);
  });

  test("the audit log records the actions taken in this run", async ({ page, probe, context }) => {
    await seedSession(context);
    await openConsole(page, "/console/audit/");
    await page.waitForLoadState("networkidle");
    probe.reset();

    const empty = page.getByText("No entries yet");
    const rows = page.locator("li, tr").filter({ has: page.locator("time, span") });
    const isEmpty = (await empty.count()) > 0;
    const count = await rows.count();
    await probe.check(
      "the audit log lists entries after the run created and revoked keys and inboxes",
      !isEmpty && count > 0,
      `empty state ${isEmpty}, rows ${count}`,
    );
    await probe.clean("audit log");
  });

  test("domains offers no destructive action without a confirmation", async ({ page, probe, context }) => {
    await seedSession(context);
    await openConsole(page, "/console/domains/");
    await page.waitForLoadState("networkidle");
    probe.reset();

    const dangerous = await page
      .locator("button")
      .evaluateAll((nodes) => nodes.map((n) => (n.textContent ?? "").trim()).filter((t) => /delete|remove|revoke/i.test(t)));
    await probe.check(
      "the domains screen has no unconfirmed destructive button",
      dangerous.length === 0 || (await page.locator("dialog").count()) > 0,
      dangerous.join(", "),
    );
    await probe.clean("domains screen");
  });
});
