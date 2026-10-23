import type { Metadata } from "next";
import { OnboardingFlow } from "@/components/onboarding/OnboardingFlow";
import { resolveOnboardingSession } from "@/components/onboarding/session";

export const metadata: Metadata = {
  title: "Get started",
  description: "Name your organization, pick a sending domain and invite your team in three steps.",
  robots: { index: false, follow: false },
};

export default function OnboardingPage() {
  // Session as a parameter: today anonymous, later the account of the caller.
  // The flow itself does not care which one it receives.
  const session = resolveOnboardingSession();

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-12 sm:px-6 sm:py-16">
      <OnboardingFlow session={session} />
    </div>
  );
}
