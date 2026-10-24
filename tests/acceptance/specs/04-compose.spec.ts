// 4. Compose: send from one inbox to another through the UI, then find the new
// thread in the receiving inbox.

import { test } from "../lib/evidence";
import { HAS_KEY, SKIP_NO_KEY } from "../lib/env";
import { createInbox, openConsole, poll, seedSession, unique, waitForThread } from "../lib/console";

test.describe("compose", () => {
  test.skip(!HAS_KEY, SKIP_NO_KEY);

  test("a message sent from the composer starts a thread in the receiving inbox", async ({ page, probe, context, request }) => {
    const sender = await createInbox(request, "qa-composer");
    const receiver = await createInbox(request, "qa-recipient");
    if (!sender || !receiver) {
      test.skip(true, "the API refused to create the two inboxes this spec needs");
      return;
    }

    const subject = unique("qa compose");
    await seedSession(context);
    await openConsole(page, `/console/compose/?from=${encodeURIComponent(sender.inbox_id)}`);
    await page.waitForLoadState("networkidle");
    probe.reset();

    const from = page.locator("#compose-from");
    await probe.check("the composer preselects the inbox from the link", (await from.inputValue()) === sender.inbox_id, await from.inputValue());

    await page.locator("#compose-to").fill(receiver.inbox_id);
    await page.locator("#compose-subject").fill(subject);
    await page.locator("#compose-text").fill("Sent by the acceptance suite through the console composer.");
    await page.getByRole("button", { name: "Send", exact: true }).click();

    const queued = page.getByRole("status").filter({ hasText: /queued/i });
    const ok = await queued.waitFor({ state: "visible", timeout: 25_000 }).then(() => true).catch(() => false);
    await probe.check("the composer reports the message as queued", ok, await page.locator("body").innerText().then((t) => t.slice(0, 300)));

    await probe.clean("compose send");

    const thread = await waitForThread(request, receiver.inbox_id, subject, 90_000);
    if (!thread) {
      await probe.check("the sent message arrives as a thread in the receiving inbox", false, `no thread with subject ${subject}`);
      return;
    }

    await openConsole(page, `/console/inbox/?address=${encodeURIComponent(receiver.inbox_id)}`);
    await page.waitForLoadState("networkidle");
    probe.reset();

    const row = page.locator('section[aria-label="Threads"] [role="option"]').filter({ hasText: subject });
    const shown = await row.waitFor({ state: "visible", timeout: 30_000 }).then(() => true).catch(() => false);
    await probe.check("the new thread appears in the receiving inbox", shown, subject);

    if (shown) {
      await row.click();
      const conversation = page.locator('section[aria-label="Conversation"]');
      const body = await conversation.innerText().catch(() => "");
      await probe.check(
        "opening the new thread shows the sent text",
        body.includes("acceptance suite through the console composer"),
        body.slice(0, 200),
      );
    }

    await probe.clean("new thread in the inbox");
  });
});
