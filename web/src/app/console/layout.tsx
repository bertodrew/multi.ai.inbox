import type { Metadata } from "next";
import type { ReactNode } from "react";
import { ConsoleShell } from "@/components/console/ConsoleShell";
import { assertConsoleMatchesSpec } from "@/lib/console/spec-check";

export const metadata: Metadata = {
  title: { default: "Console", template: "%s · Console" },
  description: "Manage inboxes, mail, API keys, webhooks and domains.",
  robots: { index: false, follow: false },
};

export default function ConsoleLayout({ children }: { children: ReactNode }) {
  // Runs during the static build: drift from openapi.yaml fails it.
  assertConsoleMatchesSpec();
  return <ConsoleShell>{children}</ConsoleShell>;
}
