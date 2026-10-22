import { test, expect, a11yViolations } from "../lib/fixtures";

const publicPages = [
  { name: "user login", path: "/" },
  { name: "admin login", path: "/admin" },
  { name: "domain admin login", path: "/domainadmin" },
  { name: "password reset", path: "/reset-password" },
];

for (const p of publicPages) {
  test(`@smoke ${p.name} renders without errors`, async ({ page, problems }, testInfo) => {
    const res = await page.goto(p.path);
    expect(res?.status(), "HTTP status").toBeLessThan(400);
    await expect(page.locator("body")).toBeVisible();

    const hasHorizontalScroll = await page.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect.soft(hasHorizontalScroll, "no horizontal scroll").toBe(false);

    const violations = await a11yViolations(page);
    await testInfo.attach("a11y.json", { body: JSON.stringify(violations, null, 2), contentType: "application/json" });
    expect.soft(violations, "critical/serious a11y violations").toEqual([]);
    expect.soft(problems.console, "console errors").toEqual([]);
  });
}

test("@smoke security headers on login", async ({ page }) => {
  const res = await page.goto("/");
  const h = res?.headers() ?? {};
  expect.soft(h["strict-transport-security"], "HSTS").toBeTruthy();
  expect.soft(h["x-content-type-options"], "nosniff").toBe("nosniff");
  expect.soft(h["x-frame-options"] ?? h["content-security-policy"], "clickjacking protection").toBeTruthy();
});
