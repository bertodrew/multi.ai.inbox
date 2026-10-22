import { test as base, expect, type ConsoleMessage, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

type Problems = { console: string[]; network: string[] };

export const test = base.extend<{ problems: Problems }>({
  problems: async ({ page }, use, testInfo) => {
    const problems: Problems = { console: [], network: [] };
    page.on("console", (msg: ConsoleMessage) => {
      if (msg.type() === "error") problems.console.push(msg.text());
    });
    page.on("pageerror", (err) => problems.console.push(`pageerror: ${err.message}`));
    page.on("response", (res) => {
      if (res.status() >= 400) problems.network.push(`${res.status()} ${res.url()}`);
    });
    page.on("requestfailed", (req) => {
      problems.network.push(`FAILED ${req.url()} ${req.failure()?.errorText ?? ""}`);
    });
    await use(problems);
    await testInfo.attach("problems.json", {
      body: JSON.stringify(problems, null, 2),
      contentType: "application/json",
    });
  },
});

export { expect };

export async function a11yViolations(page: Page): Promise<string[]> {
  const result = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  return result.violations
    .filter((v) => v.impact === "critical" || v.impact === "serious")
    .map((v) => `${v.impact}: ${v.id} (${v.nodes.length} nodes)`);
}

export async function login(page: Page, path: string, user: string, pass: string): Promise<void> {
  await page.goto(path);
  await page.locator("#login_user").fill(user);
  await page.locator("#pass_user").fill(pass);
  await page.locator('button[type="submit"][value="Login"]').click();
}
