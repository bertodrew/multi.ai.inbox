import { test, expect, login, a11yViolations } from "../lib/fixtures";
import { env } from "../lib/env";

test.describe("admin", () => {
  test("@smoke rejects invalid credentials with visible feedback", async ({ page }) => {
    await login(page, "/admin", env.E2E_ADMIN_USER, "definitely-wrong-password");
    await expect(page).not.toHaveURL(/\/admin\/dashboard/);
    await expect(page.locator(".alert-danger, .alert").first()).toBeVisible({ timeout: 10_000 });
  });

  test("@smoke logs in, dashboard loads, logs out", async ({ page, problems }, testInfo) => {
    await login(page, "/admin", env.E2E_ADMIN_USER, env.E2E_ADMIN_PASS);
    await expect(page).toHaveURL(/\/admin\/dashboard/, { timeout: 20_000 });
    await expect(page.locator(".username-lia").first()).toBeAttached();
    await page.waitForLoadState("networkidle");

    const violations = await a11yViolations(page);
    await testInfo.attach("a11y.json", { body: JSON.stringify(violations, null, 2), contentType: "application/json" });
    expect.soft(problems.console, "console errors on dashboard").toEqual([]);

    await page.evaluate(() => (document.getElementById("logout") as HTMLFormElement | null)?.submit());
    await page.waitForLoadState("load");
    await page.goto("/admin/dashboard");
    await expect(page).not.toHaveURL(/\/admin\/dashboard$/);
  });

  for (const section of ["/admin/mailbox", "/admin/system", "/admin/queue"]) {
    test(`read only: ${section} loads`, async ({ page, problems }) => {
      await login(page, "/admin", env.E2E_ADMIN_USER, env.E2E_ADMIN_PASS);
      await expect(page).toHaveURL(/\/admin\/dashboard/, { timeout: 20_000 });
      const res = await page.goto(section);
      expect(res?.status()).toBeLessThan(400);
      await page.waitForLoadState("networkidle");
      expect.soft(problems.console, "console errors").toEqual([]);
      expect.soft(problems.network.filter((n) => !n.includes("favicon")), "failed requests").toEqual([]);
    });
  }
});
