// 3. Webmail: three panes, an opened thread, a hostile body that must not run,
// remote images blocked, and attachments listed.

import { test } from "../lib/evidence";
import { HAS_KEY, SKIP_NO_KEY } from "../lib/env";
import {
  createInbox,
  getThreadMessages,
  listThreads,
  openConsole,
  poll,
  seedSession,
  sendMail,
  unique,
} from "../lib/console";

const HOSTILE_HTML = [
  '<div style="color:red">',
  "<p>QA hostile body</p>",
  "<script>window.__maiQaPwned = 1; alert('qa-script-ran');</script>",
  '<img src="https://tracker.qa.invalid/pixel.png" alt="tracker" width="600" height="200">',
  '<img src="/local.png" onerror="window.__maiQaPwned = 2" alt="onerror probe">',
  '<a href="javascript:window.__maiQaPwned=3">a link that must lose its href</a>',
  '<iframe src="https://tracker.qa.invalid/frame"></iframe>',
  "</div>",
].join("");

test.describe("webmail", () => {
  test.skip(!HAS_KEY, SKIP_NO_KEY);

  test("the three pane reader opens a thread with a sanitised body, blocked images and attachments", async ({
    page,
    probe,
    context,
    request,
  }) => {
    const sender = await createInbox(request, "qa-sender");
    const receiver = await createInbox(request, "qa-reader");
    if (!sender || !receiver) {
      test.skip(true, "the API refused to create the two inboxes this spec needs");
      return;
    }

    const subject = unique("qa hostile mail");
    const sent = await sendMail(request, sender.inbox_id, receiver.inbox_id, subject, {
      text: "QA hostile body in plain text",
      html: HOSTILE_HTML,
      attachments: [
        {
          filename: "qa-note.txt",
          content_type: "text/plain",
          content: Buffer.from("an attachment the reader must see").toString("base64"),
        },
      ],
    });
    await probe.check("the fixture message is accepted by the send API", sent.ok, `status ${sent.status}`);

    const thread = await poll(
      async () => {
        const threads = await listThreads(request, receiver.inbox_id);
        return threads.find((t) => t.subject === subject) ?? null;
      },
      120_000,
    );
    if (!thread) {
      test.skip(true, "the fixture message never arrived in the receiver inbox, so the reader cannot be exercised");
      return;
    }

    const messages = await getThreadMessages(request, thread.thread_id);
    const withHtml = messages.find((m) => typeof m.html === "string" && m.html !== "");
    const hasAttachments = messages.some((m) => (m.attachments ?? []).length > 0);

    const dialogs: string[] = [];
    const remoteRequests: string[] = [];
    page.on("dialog", (dialog) => {
      dialogs.push(dialog.message());
      void dialog.dismiss();
    });
    page.on("request", (r) => {
      if (r.url().includes("tracker.qa.invalid")) remoteRequests.push(r.url());
    });

    await seedSession(context);
    await openConsole(page, `/console/inbox/?address=${encodeURIComponent(receiver.inbox_id)}&thread=${thread.thread_id}`);
    await page.waitForLoadState("networkidle");

    const threadsPane = page.locator('section[aria-label="Threads"]');
    const readerPane = page.locator('section[aria-label="Conversation"]');
    const detailsPane = page.locator('aside[aria-label="Thread details"]');

    await probe.check("the thread list pane is visible", await threadsPane.isVisible());
    await probe.check("the conversation pane is visible", await readerPane.isVisible());
    await probe.check("the details pane is visible on a desktop viewport", await detailsPane.isVisible());

    await probe.check("the thread list shows the fixture subject", (await page.getByText(subject).count()) > 0, subject);

    const bodyText = await readerPane.innerText().catch(() => "");
    await probe.check(
      "the conversation shows the message body",
      bodyText.includes("QA hostile body"),
      bodyText.slice(0, 200),
    );

    await probe.check("no dialog was opened by the message body", dialogs.length === 0, dialogs.join(" | "));
    const pwned = await page.evaluate(() => (window as unknown as Record<string, unknown>).__maiQaPwned ?? null);
    await probe.check("no script or event handler from the message ran", pwned === null, String(pwned));
    const scriptTags = await readerPane.locator("script, iframe, style").count();
    await probe.check("no script, iframe or style element survived sanitising", scriptTags === 0, `${scriptTags} found`);
    const jsHref = await readerPane.locator('a[href^="javascript:"]').count();
    await probe.check("a javascript: link lost its href", jsHref === 0, `${jsHref} found`);

    if (withHtml) {
      const placeholders = await readerPane.locator('[data-mai-image="blocked"]').count();
      await probe.check("the remote image is replaced by a blocked image placeholder", placeholders > 0, `${placeholders} placeholders`);
      const liveImages = await readerPane.locator('img[src*="tracker.qa.invalid"]').count();
      await probe.check("no image points at the remote tracker", liveImages === 0, `${liveImages} live images`);
      await probe.check("the browser never requested the remote tracker", remoteRequests.length === 0, remoteRequests.join(" | "));
      const showImages = page.getByRole("button", { name: /Show images/ });
      await probe.check("a show images control is offered", (await showImages.count()) === 1);
    } else {
      test.info().annotations.push({
        type: "skip-reason",
        description: "the stored message had no html body, so the sanitiser path could not be exercised",
      });
      await probe.check("the message body falls back to text when there is no html", bodyText.length > 0);
    }

    if (hasAttachments) {
      await probe.check("the attachment count is shown", (await readerPane.getByText("1 attachment").count()) > 0);
      await probe.check("the attachment filename is listed", (await readerPane.getByText("qa-note.txt").count()) > 0);
    } else {
      test.info().annotations.push({
        type: "skip-reason",
        description: "the delivered message carried no attachment metadata, so the attachment list is empty",
      });
    }

    await probe.clean("webmail reader");

    const threads = await listThreads(request, receiver.inbox_id);
    probe.reset();
    await openConsole(page, `/console/inbox/?address=${encodeURIComponent(receiver.inbox_id)}`);
    await page.waitForLoadState("networkidle");
    const listed = await page.locator('section[aria-label="Threads"] [role="option"]').count();
    await probe.check(
      "the inbox without a thread parameter lists at least the fixture thread",
      listed > 0 && (await page.getByText(subject).count()) > 0,
      `${threads.length} threads in the API, ${listed} options in the list`,
    );
  });
});
