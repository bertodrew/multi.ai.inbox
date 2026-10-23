// Where the onboarding session comes from.
//
// Authentication is not wired yet. The routes resolve a session here and pass
// it down as a prop, so the funnel never assumes a signed in visitor and never
// has to be rewritten when sign in ships: only this function changes.

import type { OnboardingSession } from "./types";

export const ANONYMOUS_SCOPE = "anonymous";

/** Storage namespace for a session: per organization once there is one. */
export function scopeFor(organizationId?: string): string {
  return organizationId ? `org:${organizationId}` : ANONYMOUS_SCOPE;
}

export type SessionInput = {
  organizationId?: string;
  email?: string;
};

/**
 * The session for an onboarding request.
 *
 * Today the site is static (next export) and accounts do not exist, so this
 * returns an anonymous session with its own storage scope: the funnel works,
 * answers stay in the browser, and nothing is claimed to be saved server side.
 *
 * TODO(auth): when sign in exists, read the session on the server (cookie or
 * bearer header), then call this with the organization id:
 *
 *   const session = resolveOnboardingSession({ organizationId, email });
 *
 * The call sites in web/src/app/onboarding/page.tsx and
 * web/src/app/onboarding/done/page.tsx do not change.
 */
export function resolveOnboardingSession(input?: SessionInput): OnboardingSession {
  if (input?.organizationId) {
    return {
      status: "authenticated",
      scope: scopeFor(input.organizationId),
      organizationId: input.organizationId,
      email: input.email,
    };
  }
  return { status: "anonymous", scope: ANONYMOUS_SCOPE };
}
