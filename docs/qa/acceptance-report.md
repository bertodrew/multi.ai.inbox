# Acceptance report, multi.ai.inbox console

Run on 24 October 2026 at 09:55 CEST against staging.

| Item | Value |
| --- | --- |
| Site under test | https://162-55-213-24.sslip.io |
| Console entry | /console/ |
| Branch | fix/qa-hardening, commit ed2b5030 |
| Suite | tests/acceptance, Playwright 1.56.1 with axe 4.10.2 |
| Run | 26 specs: 6 passed, 5 failed, 15 skipped |
| Key material | no API key was present in the environment, so every spec that needs a session skipped |

Command that produced this report:

```
cd tests/acceptance && npx playwright test
```

Command that runs the whole suite, including the 15 specs that need an organization key:

```
cd tests/acceptance
MAI_E2E_API_KEY=<organization key> MAI_E2E_BASE=https://162-55-213-24.sslip.io npx playwright test
```

The suite never writes the key to a file, never prints it, and puts it only into
the tab's sessionStorage, which is what the console itself does.

## What the run proved

The 6 passing specs cover the gate without a session, a rejected key, the
onboarding funnel end to end, the web app manifest with every icon, the service
worker registration, and the offline behaviour of the console with the network
cut. The 4 defects below are proven with a failing assertion, a screenshot and
the console or network evidence captured at the moment of failure.

The run wrote 9 finding files for those 4 defects: defect 1 caught four
assertions (wrong form action, refused submission, nothing visible, a console
error) and defect 2 caught two, one per provider, because each check fails
independently. The findings are grouped here by root cause.

## Defects

### 1. The emailed sign in link cannot be submitted, the content security policy refuses it

- Severity: blocking
- Where: /login/, the form that posts to /v0/auth/magic-link
- Spec: tests/acceptance/specs/10-signin.spec.ts, "the emailed link form posts to the site origin and is allowed by the CSP"
- Steps to reproduce:
  1. Open https://162-55-213-24.sslip.io/login/
  2. Type any address in the work email field
  3. Click "Email me a sign in link"
- Expected: the browser submits the form to the API, the page moves on, and the visitor is told the link is on its way.
- Actual: nothing happens at all. The browser refuses the submission before any request leaves the page, and the console holds `Refused to send form data to 'https://api.162-55-213-24.sslip.io/v0/auth/magic-link' because it violates the following Content Security Policy directive: "form-action 'self'".` The visitor gets no error and no feedback, so the button looks broken.
- Root cause: the page is served from the site origin with `form-action 'self'`, but the form action is built from the hardcoded API host `https://api.162-55-213-24.sslip.io`, a different origin.
- Suspected files: `web/src/app/login/page.tsx` line 41, which uses `API_URL` from `web/src/lib/site.ts`, and the CSP set at the edge in `deploy/`. The console gate already solved the same problem by talking to its own origin (`web/src/lib/console/session.ts`, `defaultApiBase`); the login page did not get the same treatment.
- Evidence: `tests/acceptance/test-results/defects/*the-magic-link-form-posts-to-the-same-origin*.json` and `*the-submission-is-not-refused*.json`, with screenshots next to them.

### 2. Google, GitHub and the emailed link are all advertised but user authentication is switched off on this deployment

- Severity: high
- Where: /login/, the "Continue with Google" and "Continue with GitHub" buttons, and the magic link endpoint behind them
- Spec: tests/acceptance/specs/10-signin.spec.ts, "the OAuth start endpoints answer instead of reporting the provider is missing"
- Steps to reproduce:
  1. Open https://162-55-213-24.sslip.io/login/
  2. Click "Continue with Google" or "Continue with GitHub"
  3. Open https://162-55-213-24.sslip.io/v0/auth/oauth/google/start?redirect=1 directly
- Expected: a redirect to the provider, or the page hides the provider it cannot offer.
- Actual: both endpoints answer 501 with `{"name":"Not Implemented","code":"auth_disabled","message":"user authentication is not configured; set MAI_AUTH_SECRET to at least 32 bytes"}`. A visitor who follows the button lands on a raw JSON error page. The magic link endpoint answers 303 today, so it would work once it can be submitted, but the page still offers no working human sign in path: only the API key gate on /console/ works.
- Suspected files: `internal/core/users.go` line 282 (`provider_not_configured`) and line 86 (`auth_disabled`), plus the staging configuration in `deploy/k8s/staging`. This is a deployment gap, but the product still ships a sign in page that cannot sign anyone in, and it does not detect or label that state.
- Evidence: `tests/acceptance/test-results/defects/*continue-with-google*.json` and `*continue-with-github*.json`.

### 3. The home page code block scrolls sideways but cannot be reached with a keyboard

- Severity: medium
- Where: the marketing home page, the API example block
- Spec: tests/acceptance/specs/08-responsive-a11y.spec.ts, both the 390x844 sweep and the 1440 sweep
- Steps to reproduce:
  1. Open https://162-55-213-24.sslip.io/
  2. Run axe with the wcag2a and wcag2aa tags, or tab through the page with a keyboard
- Expected: a region that scrolls horizontally is focusable, so a keyboard user can scroll it (WCAG 2.1.1).
- Actual: axe reports `scrollable-region-focusable`, impact serious, one node: `<pre class="overflow-x-auto p-4 text-[13px] leading-relaxed">`. The element has no tabindex, so its overflowed content is unreachable without a pointer.
- Suspected files: `web/src/components/CodeTabs.tsx` lines 30 and 83, and `web/src/components/CodeBlock.tsx` line 13, which share the same pattern. A `tabindex="0"` plus `role="region"` and a label fixes all three at once.
- Evidence: `tests/acceptance/test-results/defects/*axe-finds-critical-or-serious-violations-on-home*.json`, two files, one per viewport.

### 4. Onboarding: the organization name error is not associated with the field

- Severity: low
- Where: /onboarding/, step one, the organization name field
- Spec: tests/acceptance/specs/07-onboarding.spec.ts, "an empty name is refused before step two"
- Steps to reproduce:
  1. Open https://162-55-213-24.sslip.io/onboarding/ with an empty browser profile
  2. Click "Continue" without typing a name
- Expected: the field is marked invalid and points at its error text, so a screen reader reads the message with the field.
- Actual: the input gets `aria-invalid="true"` but no `aria-describedby`, and the error paragraph has no id, so nothing ties the message to the control. The message is rendered in a `role="alert"` paragraph, which announces it once, but returning to the field later gives no explanation.
- Suspected files: `web/src/components/onboarding/parts.tsx`, the `Field` component, which renders the error paragraph and does not wire `aria-describedby` for either the hint or the error.
- Evidence: `tests/acceptance/test-results/defects/*the-validation-message-is-announced*.json`.

## What this run did not prove

15 specs were skipped because `MAI_E2E_API_KEY` is not set in the environment this run used. They are written and they assert real behaviour; they simply cannot open a session without an organization key. Nothing in this report should be read as a pass for them.

| Spec | Skipped tests |
| --- | --- |
| 01-gate.spec.ts | a stored session opens the inbox list |
| 02-inboxes.spec.ts | list, create and intake toggle; list renders what the API returns |
| 03-webmail.spec.ts | three panes, hostile body sanitised, remote images blocked, attachments listed |
| 04-compose.spec.ts | send from the composer and find the new thread |
| 05-tickets.spec.ts | counters, filters, state, priority, note, reply, timeline, closed offers only Reopen |
| 06-resources.spec.ts | keys, webhooks, domains, audit and review load; every destructive dialog confirms; the audit log records the run |
| 08-responsive-a11y.spec.ts | the console screens at 390x844, the mobile navigation targets, the console screens at 1440 |

Skipped reasons are also written into the run output, one line per test, so a run with a key shows immediately what changed.

Two further paths are marked in the specs themselves as conditional, and would be reported as skips rather than passes if staging cannot satisfy them:

- The webmail spec needs a delivered inbound message with an html body and an attachment. The fixture is sent through the API to an inbox this run creates; if staging never delivers it, the spec skips with that reason instead of asserting against an empty mailbox.
- The tickets spec needs a ticket to appear from an inbound message on an inbox with intake on, because a ticket is created from a thread. If no ticket appears, the spec skips with that reason.

## Notes for whoever runs this next

- Test data is prefixed `qa-` in every address, key name and webhook url. The inboxes created by the webmail, compose and ticket specs are left in place so the parent can inspect them; the inboxes used for the create and delete dialog checks are removed at the end of the run.
- The offline spec does not use Playwright `setOffline`, because that switch still lets the service worker and the http cache reach the origin, which makes a broken offline path look healthy. It starts a local proxy and kills it (`tests/acceptance/lib/offline.ts`), so the worker really sees failed fetches.
- Every failed expectation writes one json file under `tests/acceptance/test-results/defects/` with the assertion, the console errors, the responses at or above 400, and the screenshot path. Screenshots sit in `tests/acceptance/test-results/defects/screenshots/`.
