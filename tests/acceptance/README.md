# Acceptance suite for the multi.ai.inbox console

A browser suite that drives the deployed console and reports what is broken. It
does not start a server, it does not fix product code, and it never writes a key
to disk.

## What it needs

| Variable | Meaning |
| --- | --- |
| `MAI_E2E_BASE` | Site origin that serves the console and proxies `/v0`. Defaults to `https://162-55-213-24.sslip.io`. |
| `MAI_E2E_API_KEY` | An organization API key for staging. When it is missing every spec that needs a session skips with a reason instead of failing. |

The key is read from `process.env` and put only into the browser tab's
`sessionStorage`, which is what the console itself does. Nothing prints it.

## Install

The Playwright and axe packages come from the existing install in `tests/e2e`:

```
cd tests/e2e && npm ci
ln -sfn ../e2e/node_modules ../acceptance/node_modules
cd ../acceptance && npx playwright install chromium
```

`tests/acceptance/node_modules` is a symlink, kept out of git, so there is one
copy of the packages rather than two.

## Run

```
cd tests/acceptance
MAI_E2E_API_KEY=<org key> MAI_E2E_BASE=https://162-55-213-24.sslip.io npx playwright test
```

Without the key:

```
cd tests/acceptance && npx playwright test
```

## Where the evidence lands

- `test-results/results.json` the machine readable run
- `test-results/defects/*.json` one file per failed expectation: the assertion,
  the console errors, the responses at or above 400, and the screenshot path
- `test-results/defects/screenshots/*.png`
- `docs/qa/acceptance-report.md` the human report, numbered defects with severity

## Specs

| File | Covers |
| --- | --- |
| `01-gate.spec.ts` | the sign in gate without a session, a rejected key, and the inbox list with a session |
| `02-inboxes.spec.ts` | list, create, intake toggle, no failed request |
| `03-webmail.spec.ts` | three panes, hostile body sanitised, remote images blocked, attachments |
| `04-compose.spec.ts` | send from the composer, new thread in the receiving inbox |
| `05-tickets.spec.ts` | counters, filters, state, priority, note, reply, timeline, closed offers only Reopen |
| `06-resources.spec.ts` | keys, webhooks, domains, audit, review, and every destructive dialog |
| `07-onboarding.spec.ts` | three steps, reload keeps the draft, finish lands on the checklist |
| `08-responsive-a11y.spec.ts` | the console at 390x844, horizontal overflow, axe wcag2a and wcag2aa |
| `09-pwa.spec.ts` | manifest, service worker, offline gate and offline fallback |
| `10-signin.spec.ts` | the human sign in page: emailed link and OAuth |

## Notes

- The offline spec removes the network with a local proxy that stops answering
  (`lib/offline.ts`). Playwright's `setOffline` is not enough: it still lets the
  service worker and the http cache reach the origin, which hides the fallback.
- Ticket fixtures come from inbound mail, because a ticket is created from a
  thread. If staging never delivers the fixture message the spec skips with the
  reason rather than pretending the queue is empty.
- Everything created on staging is prefixed `qa-`.
