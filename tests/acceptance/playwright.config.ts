import { defineConfig } from "@playwright/test";
import { BASE_URL } from "./lib/env";

// The acceptance suite drives the deployed console. It talks to staging, it
// never starts a local server, and it never needs the API key to be present:
// every spec that needs a session skips with a reason when the key is missing.
export default defineConfig({
  testDir: "./specs",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 120_000,
  expect: { timeout: 12_000 },
  reporter: [["list"], ["./lib/skip-reporter.ts"], ["json", { outputFile: "test-results/results.json" }]],
  use: {
    baseURL: BASE_URL,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: "off",
    actionTimeout: 20_000,
    navigationTimeout: 40_000,
  },
  projects: [{ name: "console", use: { browserName: "chromium", viewport: { width: 1440, height: 900 } } }],
});
