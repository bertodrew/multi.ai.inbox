import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "@/components/console/kit";
import { InboxWorkspace } from "@/components/console/inbox/InboxWorkspace";

export const metadata: Metadata = { title: "Inbox" };

// useSearchParams needs a Suspense boundary in a static export.
export default function InboxPage() {
  return (
    <Suspense fallback={<LoadingState label="Opening the inbox" />}>
      <InboxWorkspace />
    </Suspense>
  );
}
