import "./globals.css";
import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";
import { SiteFooter } from "@/components/SiteFooter";
import { SiteHeader } from "@/components/SiteHeader";
import { INDEXABLE, SITE_NAME, SITE_URL } from "@/lib/site";
import { inter, jetbrainsMono, spaceGrotesk } from "./fonts";

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: { default: `${SITE_NAME}: inboxes for AI agents, hosted in Europe`, template: `%s · ${SITE_NAME}` },
  description:
    "Email inboxes for AI agents with a REST API, MCP server and signed webhooks. Hosted in the EU, on servers in Germany.",
  robots: INDEXABLE ? { index: true, follow: true } : { index: false, follow: false },
  openGraph: { siteName: SITE_NAME, type: "website" },
  // Installable from the browser: the manifest carries the name, colors and
  // icons, the apple tags carry what iOS reads instead of the manifest.
  manifest: "/manifest.webmanifest",
  applicationName: `${SITE_NAME} console`,
  icons: {
    icon: [
      { url: "/icons/icon-192.png", sizes: "192x192", type: "image/png" },
      { url: "/icons/icon-512.png", sizes: "512x512", type: "image/png" },
    ],
    apple: [{ url: "/icons/apple-touch-icon.png", type: "image/png" }],
  },
  appleWebApp: { capable: true, title: "inbox", statusBarStyle: "default" },
  // Next emits mobile-web-app-capable for the manifest aware browsers; iOS
  // still reads the prefixed name for its own standalone mode.
  other: { "apple-mobile-web-app-capable": "yes" },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#f7f6f2" },
    { media: "(prefers-color-scheme: dark)", color: "#0d1117" },
  ],
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" className={`${inter.variable} ${spaceGrotesk.variable} ${jetbrainsMono.variable}`}>
      <body className="flex min-h-dvh flex-col antialiased">
        <SiteHeader />
        <main id="main" className="flex-1">
          {children}
        </main>
        <SiteFooter />
      </body>
    </html>
  );
}
