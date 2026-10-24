// Environment for the acceptance suite.
//
// Nothing here prints, logs or persists the API key. It is read from
// process.env at run time and only ever goes into the browser sessionStorage
// of the tab under test, which is what the console itself does.

/** The site origin that serves the console and proxies /v0 and /v1. */
export const BASE_URL = (process.env.MAI_E2E_BASE ?? "https://162-55-213-24.sslip.io").replace(/\/+$/, "");

/** The organization key. Empty means every authenticated spec skips. */
const rawKey = process.env.MAI_E2E_API_KEY ?? "";
export const API_KEY = rawKey.trim();
export const HAS_KEY = API_KEY !== "";

/** The console keeps the session under this key (web/src/lib/console/session.ts). */
export const SESSION_KEY = "mai.console.session";

export const SKIP_NO_KEY =
  "MAI_E2E_API_KEY is not set, so no organization session can be opened. Run with " +
  "MAI_E2E_API_KEY=<org key> MAI_E2E_BASE=" +
  BASE_URL +
  " npx playwright test";

/** Addresses used to build fixtures. Anything created is prefixed so staging stays tidy. */
export const RUN_ID = `qa-${Date.now().toString(36)}`;
