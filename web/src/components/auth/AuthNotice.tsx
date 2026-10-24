"use client";

import { useEffect, useRef } from "react";

/**
 * Shows why a sign in attempt came back without a session.
 *
 * The API redirects to this page with ?error=<code>. The text is written into
 * an element that starts empty and hidden, so the static export and the
 * hydrated page agree, and no state is set during the effect.
 */
const MESSAGES: Record<string, string> = {
  auth_disabled:
    "Sign in is not enabled on this deployment yet. Paste an API key in the console, or ask for one and we will set you up.",
  invalid_magic_link: "That link is no longer valid. Request a new one.",
  invalid_state: "The sign in attempt expired. Try again from this page.",
  code_rejected: "The provider refused the sign in. Try another method.",
  email_not_verified: "The provider did not confirm this address. Try another method.",
  provider_not_configured: "That provider is not configured on this deployment yet.",
  provider_unavailable: "The provider is not answering right now. Try again in a minute.",
  session_revoked: "This session was signed out. Sign in again.",
};

export function AuthNotice() {
  const ref = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    const code = new URLSearchParams(window.location.search).get("error");
    const node = ref.current;
    if (!node || !code) return;
    node.textContent = MESSAGES[code] ?? "The sign in did not complete. Try again.";
    node.hidden = false;
  }, []);

  return (
    <p
      ref={ref}
      hidden
      role="alert"
      className="mb-5 rounded-lg border border-border border-l-4 border-l-danger bg-card px-3 py-2 text-sm"
    />
  );
}
