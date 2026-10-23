import type { Metadata } from "next";
import { ButtonLink, Container, Eyebrow } from "@/components/ui";

// Served by the service worker for a navigation it cannot answer from the
// network or the shell cache. It is deliberately empty of mail: a cached page
// must never look like a live inbox.
export const metadata: Metadata = {
  title: "Offline",
  robots: { index: false, follow: false },
};

export default function OfflinePage() {
  return (
    <Container className="py-24">
      <Eyebrow>Offline</Eyebrow>
      <h1 className="mt-3 font-display text-4xl font-semibold tracking-tight">The console is offline</h1>
      <p className="mt-4 max-w-xl text-lg text-muted">
        This page was stored on your device so the app can open without a connection. Mail is not stored on the device,
        so nothing here is a copy of your inbox. Reconnect and open the console to read it.
      </p>
      <p className="mt-4 max-w-xl text-muted">
        Signing in needs a connection too: the console checks the key against the API before it opens anything.
      </p>
      <div className="mt-8 flex flex-wrap gap-3">
        <ButtonLink href="/console/">Open the console again</ButtonLink>
        <ButtonLink href="/" variant="secondary">
          Home
        </ButtonLink>
      </div>
    </Container>
  );
}
