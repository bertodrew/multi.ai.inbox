"use client";

import { useEffect } from "react";

/**
 * Picks up the tokens the API puts in the URL fragment after a sign in.
 *
 * The fragment never reaches the server, so the tokens are not in any access
 * log; this component reads them, stores them in the same place the console
 * keeps an API key, and replaces the history entry so a reload does not carry
 * them again. A session token is accepted wherever an API key is, which is why
 * the console needs no second code path.
 */
export function SessionFromHash() {
  useEffect(() => {
    const raw = window.location.hash.replace(/^#/, "");
    if (!raw) return;
    const params = new URLSearchParams(raw);
    const accessToken = params.get("access_token");
    if (!accessToken) return;
    try {
      window.sessionStorage.setItem(
        "mai.console.session",
        JSON.stringify({ apiKey: accessToken, apiBase: window.location.origin }),
      );
    } catch {
      return; // storage disabled: the sign in page says to use an API key
    }
    window.location.replace("/console/");
  }, []);

  return null;
}
