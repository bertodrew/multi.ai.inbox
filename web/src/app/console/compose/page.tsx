import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "@/components/console/kit";
import { ComposeView } from "@/components/console/ComposeView";

export const metadata: Metadata = { title: "Compose" };

// useSearchParams needs a Suspense boundary in a static export.
export default function ComposePage() {
  return (
    <Suspense fallback={<LoadingState label="Loading" />}>
      <ComposeView />
    </Suspense>
  );
}
