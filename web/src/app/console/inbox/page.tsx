import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "@/components/console/kit";
import { InboxView } from "@/components/console/InboxView";

export const metadata: Metadata = { title: "Inbox" };

// useSearchParams needs a Suspense boundary in a static export.
export default function InboxPage() {
  return (
    <Suspense fallback={<LoadingState label="Loading" />}>
      <InboxView />
    </Suspense>
  );
}
