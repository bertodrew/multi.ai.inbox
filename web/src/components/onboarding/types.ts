// Shapes and pure helpers for the onboarding funnel (route: web/src/app/onboarding).
//
// The session is a parameter, not a prerequisite: every component below takes
// the session it was given and namespaces its storage with it. Nothing here
// reads cookies, hits an API or redirects to a sign in page, so the funnel
// renders today, before authentication exists, and keeps working when it lands.

export type OnboardingSession =
  | { status: "anonymous"; scope: string }
  | { status: "authenticated"; scope: string; organizationId: string; email?: string };

/** Bump when the stored draft shape changes: older drafts are then dropped. */
export const DRAFT_VERSION = 1;
/** Bump when the checklist shape changes. */
export const CHECKLIST_VERSION = 1;

export const MAX_INVITES = 10;
export const MAX_ORG_NAME = 80;

export type DomainChoice = "own" | "test";

export type OnboardingStep = 1 | 2 | 3;

export const STEP_COUNT = 3;

export const STEP_LABELS = ["Organization", "Sending domain", "Your team"] as const;

export type OnboardingDraft = {
  orgName: string;
  domainChoice: DomainChoice | null;
  domain: string;
  invites: string[];
  step: OnboardingStep;
  completedAt: string | null;
  updatedAt: string;
};

export function emptyDraft(): OnboardingDraft {
  return {
    orgName: "",
    domainChoice: null,
    domain: "",
    invites: [],
    step: 1,
    completedAt: null,
    updatedAt: "",
  };
}

/** True when the draft holds an answer worth persisting. */
export function draftHasContent(draft: OnboardingDraft): boolean {
  return (
    draft.orgName.trim().length > 0 ||
    draft.domainChoice !== null ||
    draft.domain.length > 0 ||
    draft.invites.length > 0 ||
    draft.step !== 1 ||
    draft.completedAt !== null
  );
}

export type OnboardingSubmitResult = { ok: true } | { ok: false; error: string };

/**
 * The write that turns a draft into an organization. Left undefined the funnel
 * only stores the draft locally, which is the behaviour today: there is no
 * account API to call yet.
 */
export type OnboardingSubmit = (
  draft: OnboardingDraft,
  session: OnboardingSession,
) => Promise<OnboardingSubmitResult> | OnboardingSubmitResult;

export const CHECKLIST_IDS = ["first-inbox", "verify-domain", "first-email", "connect-agent"] as const;

export type ChecklistItemId = (typeof CHECKLIST_IDS)[number];

export type ChecklistItem = {
  id: ChecklistItemId;
  title: string;
  detail: string;
  href: string;
  cta: string;
};

export type ChecklistState = {
  done: ChecklistItemId[];
  dismissed: boolean;
};

export function emptyChecklist(): ChecklistState {
  return { done: [], dismissed: false };
}

/**
 * The four steps after setup. The domain entry follows the answer given in
 * step 2, so a free test address is never asked to publish DNS records.
 */
export function checklistItems(draft: OnboardingDraft | null): ChecklistItem[] {
  const ownDomain = draft?.domainChoice === "own" ? draft.domain.trim() : "";
  const verifyDetail = ownDomain
    ? `Publish the four DNS records for ${ownDomain}. Checks run every five minutes for seven days, then the domain passes on its own.`
    : "A free test address needs no DNS records. Add a domain you own when you want to send from your own name, then publish four records.";

  return [
    {
      id: "first-inbox",
      title: "Create your first inbox",
      detail: "One call or one click gives an agent its own address. Without a domain it lands on the shared test domain.",
      href: "/console/inbox/",
      cta: "Open inboxes",
    },
    {
      id: "verify-domain",
      title: "Verify your domain",
      detail: verifyDetail,
      href: "/console/domains/",
      cta: "Open domains",
    },
    {
      id: "first-email",
      title: "Send or receive the first email",
      detail: "Reply from the console or with one POST. Threads arrive parsed, with the quoted history already stripped.",
      href: "/console/compose/",
      cta: "Write a message",
    },
    {
      id: "connect-agent",
      title: "Connect an agent over MCP",
      detail: "Point Claude Code, Cursor or Codex at /mcp with an API key and the inbox tools show up as tools, not as HTTP calls.",
      href: "/docs/mcp/",
      cta: "Read the MCP guide",
    },
  ];
}

/** Strips protocol, path and a leading www, then lowercases. */
export function normalizeDomain(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/\/.*$/, "")
    .replace(/^www\./, "");
}

/** A domain with at least one dot, labels of 1 to 63 characters, no underscores,
 *  a top level label of 2 or more letters, and no bare IP address. */
export function isValidDomain(raw: string): boolean {
  const domain = normalizeDomain(raw);
  if (domain.length < 4 || domain.length > 253) return false;
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/.test(domain)) return false;
  const tld = domain.split(".").at(-1) ?? "";
  return tld.length >= 2 && !/^[0-9]+$/.test(tld);
}

export function isValidEmail(raw: string): boolean {
  const email = raw.trim();
  if (email.length < 5 || email.length > 254) return false;
  return /^[^\s@]+@[^\s@]+\.[a-z0-9-]{2,}$/i.test(email);
}

/** Message shown under the step, or null when the step can be submitted. */
export function validateStep(step: OnboardingStep, draft: OnboardingDraft): string | null {
  if (step === 1) {
    const name = draft.orgName.trim();
    if (name.length < 2) return "Enter the name of your organization, at least 2 characters.";
    if (name.length > MAX_ORG_NAME) return `Keep the name under ${MAX_ORG_NAME} characters.`;
    return null;
  }
  if (step === 2) {
    if (draft.domainChoice === null) return "Choose how you want to send your first email.";
    if (draft.domainChoice === "own" && !isValidDomain(draft.domain)) {
      return "Enter a domain like example.com. A full URL works too: only the host is kept.";
    }
    return null;
  }
  return null;
}
