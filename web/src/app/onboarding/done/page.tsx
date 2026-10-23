import type { Metadata } from "next";
import { Arrival } from "@/components/onboarding/Arrival";
import { resolveOnboardingSession } from "@/components/onboarding/session";

export const metadata: Metadata = {
  title: "Workspace checklist",
  description: "The four steps from an empty workspace to an agent with its own inbox.",
  robots: { index: false, follow: false },
};

export default function OnboardingDonePage() {
  const session = resolveOnboardingSession();

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-12 sm:px-6 sm:py-16">
      <Arrival session={session} />
    </div>
  );
}
