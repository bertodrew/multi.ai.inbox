// 10. Sign in: the human sign in page. It advertises Google, GitHub and an
// emailed link, and it must actually be able to submit. No API key is needed.

import { test } from "../lib/evidence";
import { BASE_URL } from "../lib/env";

function originOf(url: string): string {
  // A relative action is the better shape here, because it follows whichever
  // host served the page. It resolves against the site, not against nothing.
  try {
    return new URL(url, BASE_URL).origin;
  } catch {
    return url;
  }
}

test.describe("sign in", () => {
  test("the emailed link form posts to the site origin and is allowed by the CSP", async ({ page, probe }) => {
    await page.goto(`${BASE_URL}/login/`, { waitUntil: "networkidle" });
    probe.reset();

    const form = page.locator("form").first();
    const action = (await form.getAttribute("action")) ?? "";
    await probe.check(
      "the magic link form posts to the same origin that serves the page",
      originOf(action) === new URL(BASE_URL).origin,
      `action ${action}`,
    );

    await page.locator("#email").fill("qa-acceptance@example.invalid");
    const before = page.url();
    await page
      .getByRole("button", { name: "Email me a sign in link" })
      .click({ noWaitAfter: true })
      .catch(() => undefined);
    await page.waitForTimeout(3_000);

    const refused = probe.consoleErrors.some((text) => /content security policy/i.test(text) && /form-action/i.test(text));
    await probe.check("the submission is not refused by the content security policy", !refused, probe.consoleErrors.join(" | "));

    const navigated = page.url() !== before;
    // A deployment without MAI_AUTH_SECRET answers auth_disabled. That is a
    // configuration gap recorded in docs/qa/production-readiness.md, not a
    // browser defect, so it is reported as such and not as a pass.
    const authDisabled = /auth_disabled/.test(page.url());
    if (authDisabled && !refused) {
      test.skip(true, "user authentication is disabled on this deployment (no MAI_AUTH_SECRET): the form is allowed, the endpoint says auth_disabled");
    }
    const confirmation = await page.getByText(/check your inbox|link is on its way|sent/i).count();
    await probe.check(
      "submitting the form does something visible: a navigation or a confirmation",
      navigated || confirmation > 0,
      `url ${page.url()}, confirmation nodes ${confirmation}`,
    );

    await probe.clean("sign in page");
  });

  test("the OAuth start endpoints answer instead of reporting the provider is missing", async ({ request, probe }) => {
    test.info().annotations.push({
      type: "note",
      description: "a 501 here means the provider is not configured on staging, which leaves only the API key gate",
    });

    const unconfigured: string[] = [];
    for (const provider of ["google", "github"]) {
      const response = await request.get(`${BASE_URL}/v0/auth/oauth/${provider}/start?redirect=1`, {
        maxRedirects: 0,
        failOnStatusCode: false,
      });
      if (response.status() === 501) {
        unconfigured.push(provider);
        continue;
      }
      await probe.check(
        `continue with ${provider} starts an authorization instead of answering ${response.status()}`,
        response.status() < 400,
        `status ${response.status()}`,
      );
    }
    if (unconfigured.length > 0) {
      test.skip(
        true,
        `no OAuth credentials on this deployment for ${unconfigured.join(" and ")}: that is a configuration gap, recorded in docs/qa/production-readiness.md`,
      );
    }
  });
});
