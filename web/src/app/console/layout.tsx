import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";
import { ConsoleShell } from "@/components/console/ConsoleShell";
import { ServiceWorkerRegistrar } from "@/components/pwa/ServiceWorkerRegistrar";
import { assertConsoleMatchesSpec } from "@/lib/console/spec-check";

export const metadata: Metadata = {
  title: { default: "Console", template: "%s · Console" },
  description: "Manage inboxes, mail, API keys, webhooks and domains.",
  robots: { index: false, follow: false },
};

// The installed app draws its status bar and title bar in the ink color, which
// matches theme_color in the manifest so the frame does not flash on launch.
export const viewport: Viewport = { themeColor: "#14161a" };

export default function ConsoleLayout({ children }: { children: ReactNode }) {
  // Runs during the static build: drift from openapi.yaml fails it.
  assertConsoleMatchesSpec();
  return (
    <>
      {/* Registered on the sign in screen too: the shell has to be cached
          before anyone is signed in for an offline launch to reach the gate. */}
      <ServiceWorkerRegistrar />
      <ConsoleShell>{children}</ConsoleShell>
    </>
  );
}
