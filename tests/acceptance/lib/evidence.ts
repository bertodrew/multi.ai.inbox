// Evidence collection for the acceptance suite.
//
// Every failed expectation is written as one JSON finding: the assertion, the
// console errors, the network responses at or above 400, and a screenshot.
// Checks are soft so one broken screen does not hide the next one.

import { test as base, expect, type Page, type TestInfo } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export type NetFailure = { status: number; method: string; url: string };

export type Finding = {
  seq: number;
  spec: string;
  test: string;
  project: string;
  title: string;
  detail: string;
  consoleErrors: string[];
  pageErrors: string[];
  network: NetFailure[];
  screenshot: string;
  at: string;
};

const ROOT = join(process.cwd(), "test-results", "defects");
const SHOTS = join(ROOT, "screenshots");

function slug(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 80);
}

/** Noise that is not a product defect: favicon probes and cancelled navigations. */
function ignorable(url: string): boolean {
  return /\/favicon\.ico$/.test(url);
}

export class Probe {
  readonly consoleErrors: string[] = [];
  readonly pageErrors: string[] = [];
  readonly network: NetFailure[] = [];
  readonly findings: Finding[] = [];
  private page: Page;
  private testInfo: TestInfo;
  private seq = 0;

  constructor(page: Page, testInfo: TestInfo) {
    this.page = page;
    this.testInfo = testInfo;
    page.on("console", (message) => {
      if (message.type() === "error") this.consoleErrors.push(message.text());
    });
    page.on("pageerror", (error) => {
      this.pageErrors.push(error.message);
    });
    page.on("response", (response) => {
      const status = response.status();
      if (status < 400) return;
      const url = response.url();
      if (ignorable(url)) return;
      this.network.push({ status, method: response.request().method(), url });
    });
  }

  /** Console errors and page errors seen so far, without the noise of a failed favicon. */
  get errors(): string[] {
    return [...this.consoleErrors, ...this.pageErrors.map((e) => `pageerror: ${e}`)];
  }

  async shot(name: string): Promise<string> {
    const file = join(SHOTS, `${String(this.seq).padStart(2, "0")}-${slug(name)}-${slug(this.testInfo.title)}.png`);
    mkdirSync(SHOTS, { recursive: true });
    try {
      await this.page.screenshot({ path: file, fullPage: true, timeout: 10_000 });
    } catch {
      return "";
    }
    return file;
  }

  /** Records a finding and marks the test failed, without stopping the test. */
  async fail(title: string, detail = ""): Promise<void> {
    this.seq += 1;
    const screenshot = await this.shot(title);
    const finding: Finding = {
      seq: this.seq,
      spec: this.testInfo.file.split("/").slice(-1)[0] ?? "",
      test: this.testInfo.title,
      project: this.testInfo.project.name,
      title,
      detail,
      consoleErrors: [...this.consoleErrors],
      pageErrors: [...this.pageErrors],
      network: [...this.network],
      screenshot,
      at: new Date().toISOString(),
    };
    this.findings.push(finding);
    mkdirSync(ROOT, { recursive: true });
    writeFileSync(
      join(ROOT, `${this.testInfo.testId}-${this.seq}-${slug(title)}.json`),
      JSON.stringify(finding, null, 2),
      "utf8",
    );
    expect.soft(false, detail === "" ? title : `${title}: ${detail}`).toBe(true);
  }

  /** A named expectation. Returns true when it held. */
  async check(title: string, ok: boolean, detail = ""): Promise<boolean> {
    if (ok) return true;
    await this.fail(title, detail);
    return false;
  }

  /** Fails when a check threw instead of returning false. */
  async guard(title: string, fn: () => Promise<boolean>): Promise<boolean> {
    try {
      return await fn();
    } catch (error) {
      await this.fail(title, error instanceof Error ? error.message : String(error));
      return false;
    }
  }

  /** Expects no console error and no response at or above 400 since the last reset. */
  async clean(title: string): Promise<void> {
    const errors = this.errors;
    const network = this.network;
    if (errors.length > 0) {
      await this.fail(`${title}: console errors`, errors.join(" | "));
      this.consoleErrors.length = 0;
      this.pageErrors.length = 0;
    }
    if (network.length > 0) {
      await this.fail(
        `${title}: responses at or above 400`,
        network.map((n) => `${n.method} ${n.status} ${n.url}`).join(" | "),
      );
      this.network.length = 0;
    }
  }

  reset(): void {
    this.consoleErrors.length = 0;
    this.pageErrors.length = 0;
    this.network.length = 0;
  }
}

export const test = base.extend<{ probe: Probe }>({
  probe: async ({ page }, use, testInfo) => {
    const probe = new Probe(page, testInfo);
    await use(probe);
    if (probe.findings.length > 0) {
      await testInfo.attach("acceptance-findings", {
        body: JSON.stringify(probe.findings, null, 2),
        contentType: "application/json",
      });
    }
  },
});

export { expect };
