// Draft and checklist persistence for the onboarding funnel.
//
// localStorage only, namespaced by the session scope, wrapped so a browser that
// refuses storage (private mode, quota, blocked cookies) degrades to an in
// memory funnel instead of throwing. Every read validates what it finds: a
// half written or stale blob returns null rather than a broken step.

import { z } from "zod";
import {
  CHECKLIST_IDS,
  CHECKLIST_VERSION,
  DRAFT_VERSION,
  emptyChecklist,
  type ChecklistItemId,
  type ChecklistState,
  type OnboardingDraft,
} from "./types";

const DraftSchema = z.object({
  version: z.literal(DRAFT_VERSION),
  orgName: z.string().max(120),
  domainChoice: z.enum(["own", "test"]).nullable(),
  domain: z.string().max(253),
  invites: z.array(z.string().max(254)).max(50),
  step: z.union([z.literal(1), z.literal(2), z.literal(3)]),
  completedAt: z.string().nullable(),
  updatedAt: z.string(),
});

const ChecklistSchema = z.object({
  version: z.literal(CHECKLIST_VERSION),
  done: z.array(z.string()).max(20),
  dismissed: z.boolean(),
});

export function draftKey(scope: string): string {
  return `mai.onboarding.draft.v${DRAFT_VERSION}.${scope}`;
}

export function checklistKey(scope: string): string {
  return `mai.onboarding.checklist.v${CHECKLIST_VERSION}.${scope}`;
}

function readJson(key: string): unknown {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(key);
    return raw === null ? null : JSON.parse(raw);
  } catch {
    return null;
  }
}

function writeJson(key: string, value: unknown): boolean {
  if (typeof window === "undefined") return false;
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
    return true;
  } catch {
    return false;
  }
}

function remove(key: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(key);
  } catch {
    // nothing stored, nothing to remove
  }
}

export function readDraft(scope: string): OnboardingDraft | null {
  const parsed = DraftSchema.safeParse(readJson(draftKey(scope)));
  if (!parsed.success) return null;
  return {
    orgName: parsed.data.orgName,
    domainChoice: parsed.data.domainChoice,
    domain: parsed.data.domain,
    invites: parsed.data.invites,
    step: parsed.data.step,
    completedAt: parsed.data.completedAt,
    updatedAt: parsed.data.updatedAt,
  };
}

/** Returns false when the browser refused to store the draft. */
export function writeDraft(scope: string, draft: OnboardingDraft): boolean {
  return writeJson(draftKey(scope), { version: DRAFT_VERSION, ...draft });
}

export function clearDraft(scope: string): void {
  remove(draftKey(scope));
}

export function readChecklist(scope: string): ChecklistState {
  const parsed = ChecklistSchema.safeParse(readJson(checklistKey(scope)));
  if (!parsed.success) return emptyChecklist();
  const known = CHECKLIST_IDS as readonly string[];
  const done = parsed.data.done.filter((id): id is ChecklistItemId => known.includes(id));
  return { done, dismissed: parsed.data.dismissed };
}

/** Returns false when the browser refused to store the checklist. */
export function writeChecklist(scope: string, state: ChecklistState): boolean {
  return writeJson(checklistKey(scope), { version: CHECKLIST_VERSION, ...state });
}

/** Drops both records for this scope, used by the start over action. */
export function clearOnboarding(scope: string): void {
  clearDraft(scope);
  remove(checklistKey(scope));
}
