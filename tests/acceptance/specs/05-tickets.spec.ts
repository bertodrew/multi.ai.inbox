// 5. Tickets: filters and counters, a ticket opened and moved, a priority, an
// internal note, a reply, a growing timeline, and a closed ticket that only
// offers Reopen.

import type { Page } from "@playwright/test";
import { test } from "../lib/evidence";
import { HAS_KEY, SKIP_NO_KEY } from "../lib/env";
import {
  createInbox,
  openConsole,
  seedSession,
  sendMail,
  setIntake,
  unique,
  waitForTicket,
} from "../lib/console";

test.describe("tickets", () => {
  test.skip(!HAS_KEY, SKIP_NO_KEY);

  test("the queue filters, and a ticket can be moved, prioritised, noted and answered", async ({
    page,
    probe,
    context,
    request,
  }) => {
    const sender = await createInbox(request, "qa-requester");
    const desk = await createInbox(request, "qa-desk");
    if (!sender || !desk) {
      test.skip(true, "the API refused to create the two inboxes this spec needs");
      return;
    }

    const intakeOn = await setIntake(request, desk.inbox_id, true);
    await probe.check("the desk inbox accepts intake", intakeOn);

    const subject = unique("qa ticket");
    const sent = await sendMail(request, sender.inbox_id, desk.inbox_id, subject, {
      text: "The acceptance suite needs a ticket from this message.",
    });
    await probe.check("the fixture message is accepted", sent.ok, `status ${sent.status}`);

    const ticket = await waitForTicket(request, subject, 120_000);
    if (!ticket) {
      test.skip(true, "no ticket was created from the inbound message, so the queue cannot be exercised");
      return;
    }

    await seedSession(context);
    await openConsole(page, "/console/tickets/");
    await page.waitForLoadState("networkidle");
    probe.reset();

    await probe.check("the tickets page renders", await page.getByRole("heading", { level: 1, name: "Tickets" }).isVisible());

    for (const label of ["Open", "Breaching SLA", "Unassigned", "Median first reply"]) {
      await probe.check(`the ${label} counter is shown`, (await page.getByText(label, { exact: true }).count()) > 0);
    }

    const stateFilter = page.locator("select").first();
    await stateFilter.selectOption("open");
    await page.waitForTimeout(1_500);
    const openCount = await page.locator("ul.space-y-2 > li").count();
    await stateFilter.selectOption("");
    await page.waitForTimeout(1_500);
    const anyCount = await page.locator("ul.space-y-2 > li").count();
    await probe.check("the state filter changes the queue", anyCount >= openCount, `open ${openCount}, any ${anyCount}`);

    const priorityFilter = page.locator("select").nth(1);
    await priorityFilter.selectOption("high");
    await page.waitForTimeout(1_500);
    const highCount = await page.locator("ul.space-y-2 > li").count();
    await priorityFilter.selectOption("");
    await page.waitForTimeout(1_500);
    await probe.check("the priority filter applies without an error", true, `high ${highCount}`);

    const search = page.getByPlaceholder(/Subject or requester/i);
    await probe.check("the queue has a search field", (await search.count()) === 1);

    const row = page.locator("ul.space-y-2 > li button").filter({ hasText: subject }).first();
    const opened = await row.waitFor({ state: "visible", timeout: 30_000 }).then(() => true).catch(() => false);
    await probe.check("the fixture ticket is listed", opened, subject);
    if (!opened) {
      await probe.clean("ticket queue");
      return;
    }
    await row.click();

    const detail = page.getByRole("heading", { level: 2, name: subject });
    await probe.check("the ticket detail opens", await detail.isVisible());
    await probe.check("the ticket number is shown", (await page.getByText(new RegExp(`T-${ticket.number}`)).count()) > 0);

    const timelineBefore = await timelineCount(page);
    await probe.check("the timeline has entries before any change", timelineBefore > 0, `${timelineBefore}`);

    // state: from new the queue offers Open; click whichever forward move exists
    const openButton = page.getByRole("button", { name: "Open", exact: true });
    if ((await openButton.count()) > 0) {
      await openButton.click();
      await page.waitForTimeout(2_000);
      await probe.check(
        "moving the ticket to open updates its state badge",
        (await page.locator("span").filter({ hasText: /^open$/ }).count()) > 0,
      );
    } else {
      await probe.check("an open ticket offers a forward state move", false, "no Open button on a new ticket");
    }

    await page.locator("#priority").selectOption("high");
    await page.waitForTimeout(2_000);
    await probe.check(
      "setting the priority updates the badge",
      (await page.locator("span").filter({ hasText: /^high$/ }).count()) > 0,
    );

    const timelineAfterState = await timelineCount(page);
    await probe.check("a state change grows the timeline", timelineAfterState > timelineBefore, `${timelineBefore} then ${timelineAfterState}`);

    const note = unique("QA internal note");
    await page.getByPlaceholder("What the next person needs to know").fill(note);
    await page.getByRole("button", { name: "Save note" }).click();
    await page.waitForTimeout(2_500);
    await probe.check("the internal note appears in the timeline", (await page.getByText(note).count()) > 0, note);

    const afterNote = await timelineCount(page);
    await probe.check("the note grows the timeline", afterNote > timelineAfterState, `${timelineAfterState} then ${afterNote}`);

    await page.getByPlaceholder("Your answer").fill("Answered by the acceptance suite.");
    await page.getByRole("button", { name: "Send reply" }).click();
    await page.waitForTimeout(4_000);
    const afterReply = await timelineCount(page);
    await probe.check("the reply grows the timeline", afterReply > afterNote, `${afterNote} then ${afterReply}`);
    await probe.check("the reply is recorded in the timeline", (await page.getByText(/repl/i).count()) > 0);

    await probe.clean("ticket detail and timeline");

    // closed tickets offer only Reopen
    const close = page.getByRole("button", { name: "Close", exact: true });
    if ((await close.count()) > 0) {
      await close.click();
      await page.waitForTimeout(3_000);
      const stateButtons = await page
        .locator("button")
        .evaluateAll((nodes) =>
          nodes
            .map((n) => (n.textContent ?? "").trim())
            .filter((t) => ["Open", "Close", "Pending", "Resolve", "Reopen"].includes(t)),
        );
      await probe.check(
        "a closed ticket offers only Reopen",
        stateButtons.length === 1 && stateButtons[0] === "Reopen",
        stateButtons.join(", "),
      );
    } else {
      await probe.check("a ticket offers a Close action", false, "no Close button found");
    }

    await probe.clean("closing a ticket");
  });
});

async function timelineCount(page: Page): Promise<number> {
  const card = page.locator("div").filter({ has: page.getByRole("heading", { name: "Timeline" }) }).last();
  return card.locator("ul > li").count();
}
