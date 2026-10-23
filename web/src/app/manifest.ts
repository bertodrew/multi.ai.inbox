import type { MetadataRoute } from "next";
import { SITE_NAME } from "@/lib/site";

// Web app manifest, emitted as a static /manifest.webmanifest by `next build`.
// Colors are the brand tokens from globals.css: theme_color is the ink color
// the console header uses, background_color is the paper color the app opens on.
export const dynamic = "force-static";

export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/console/",
    name: `${SITE_NAME} console`,
    short_name: "inbox",
    description: "Read and send mail, manage inboxes, keys, webhooks and domains.",
    lang: "en",
    dir: "ltr",
    start_url: "/console/",
    scope: "/",
    display: "standalone",
    theme_color: "#14161a",
    background_color: "#f7f6f2",
    icons: [
      { src: "/icons/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icons/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icons/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
    shortcuts: [
      { name: "Inboxes", url: "/console/" },
      { name: "Compose", url: "/console/compose/" },
      { name: "Review queue", url: "/console/review/" },
      { name: "Tickets", url: "/console/tickets/" },
    ],
  };
}
