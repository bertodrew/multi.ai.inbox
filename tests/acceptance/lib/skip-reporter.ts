// Prints one line per skipped test, with the reason, so a run without an API
// key says why it left fifteen specs alone instead of showing a wall of dashes.

import type { Reporter, TestCase, TestResult } from "@playwright/test/reporter";

export default class SkipReporter implements Reporter {
  onTestEnd(test: TestCase, result: TestResult): void {
    if (result.status !== "skipped") return;
    const reason = result.annotations
      .map((annotation) => annotation.description ?? "")
      .filter((text) => text !== "")
      .join(" ");
    console.log(`skipped: ${test.title}${reason === "" ? "" : ` because ${reason}`}`);
  }
}
