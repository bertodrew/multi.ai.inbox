// 7. Onboarding: three steps, a reload in the middle keeps the answers, and
// finishing lands on the checklist. No API key is needed: the funnel keeps its
// draft in localStorage.

import { test } from "../lib/evidence";
import { BASE_URL } from "../lib/env";
import { noHorizontalOverflow } from "../lib/console";

async function start(page: import("@playwright/test").Page): Promise<void> {
  await page.goto(`${BASE_URL}/onboarding/`);
  await page.evaluate(() => window.localStorage.clear());
  await page.reload({ waitUntil: "networkidle" });
}

test.describe("onboarding", () => {
  test("three steps, a reload keeps the draft, and finishing lands on the checklist", async ({ page, probe }) => {
    await start(page);
    probe.reset();

    await probe.check("the funnel renders its heading", await page.getByRole("heading", { level: 1, name: "Set up your workspace" }).isVisible());
    const progress = page.getByRole("navigation", { name: "Setup progress" });
    await probe.check("the progress indicator is shown", (await progress.count()) === 1);
    await probe.check("the progress says step 1 of 3", /step 1 of 3/i.test(await progress.innerText()));
    await probe.check("step one asks for the organization name", await page.getByRole("heading", { level: 2, name: "Name your organization" }).isVisible());

    const orgName = `QA Workspace ${Date.now().toString(36)}`;
    await page.getByLabel("Organization name").fill(orgName);
    await page.getByRole("button", { name: "Continue" }).click();
    await page.waitForTimeout(800);

    await probe.check("continue moves to the sending domain step", await page.getByRole("heading", { level: 2, name: "Pick a sending domain" }).isVisible());
    await probe.check("the progress says step 2 of 3", /step 2 of 3/i.test(await progress.innerText()));

    await page.getByRole("radio", { name: /Use a free test address/ }).check();
    await page.waitForTimeout(500);

    // reload in the middle: the draft has to survive
    await page.reload({ waitUntil: "networkidle" });
    await probe.check("a reload keeps the funnel on step two", await page.getByRole("heading", { level: 2, name: "Pick a sending domain" }).isVisible());
    await probe.check(
      "a reload keeps the chosen domain option",
      await page.getByRole("radio", { name: /Use a free test address/ }).isChecked(),
    );
    await probe.check("the funnel says it picked up where the visitor left off", (await page.getByText("Picked up where you left off in this browser.").count()) > 0);

    await page.getByRole("button", { name: "Continue" }).click();
    await page.waitForTimeout(800);
    await probe.check("continue moves to the team step", await page.getByRole("heading", { level: 2, name: "Invite your team" }).isVisible());
    await probe.check("the progress says step 3 of 3", /step 3 of 3/i.test(await progress.innerText()));

    await page.getByLabel("Teammate email").fill("qa.teammate@example.com");
    await page.getByRole("button", { name: "Add", exact: true }).click();
    await page.waitForTimeout(500);
    await probe.check("the invitation is listed", (await page.getByText("qa.teammate@example.com").count()) > 0);

    await page.getByRole("button", { name: "Finish setup" }).click();
    await page.waitForURL(/\/onboarding\/done\/?/, { timeout: 20_000 }).catch(() => undefined);
    const landed = /\/onboarding\/done\/?/.test(page.url());
    await probe.check("finishing lands on the checklist route", landed, page.url());
    await probe.check("the checklist is rendered", await page.getByRole("heading", { name: "Setup checklist" }).isVisible().catch(() => false));

    const overflow = await noHorizontalOverflow(page);
    await probe.check("the checklist does not scroll sideways", overflow.ok, overflow.detail);

    await probe.clean("onboarding funnel");
  });

  test("an empty name is refused before step two", async ({ page, probe }) => {
    await start(page);
    probe.reset();

    await page.getByRole("button", { name: "Continue" }).click();
    await page.waitForTimeout(600);
    await probe.check(
      "step one refuses to continue without a name",
      await page.getByRole("heading", { level: 2, name: "Name your organization" }).isVisible(),
    );
    const described = await page.getByLabel("Organization name").evaluate((el) => ({
      invalid: el.getAttribute("aria-invalid"),
      describedBy: el.getAttribute("aria-describedby"),
      described: el.getAttribute("aria-describedby") ? document.getElementById(el.getAttribute("aria-describedby") ?? "")?.textContent ?? "" : "",
    }));
    await probe.check("the field is marked invalid", described.invalid === "true", JSON.stringify(described));
    await probe.check(
      "the validation message is announced",
      (described.described ?? "").trim() !== "",
      JSON.stringify(described),
    );

    await probe.clean("onboarding validation");
  });
});
