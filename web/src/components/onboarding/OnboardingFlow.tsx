"use client";

// The three step funnel. All state lives in one draft that is written to
// storage on each keystroke, so a reload, a closed tab or a back navigation
// returns to the same step with the same answers.
//
// The session arrives as a prop and only decides the storage scope and what a
// submit is allowed to do. No sign in is required to walk through it.

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Eyebrow } from "@/components/ui";
import { useHydrated } from "@/lib/use-hydrated";
import { StepDomain } from "./StepDomain";
import { StepOrganization } from "./StepOrganization";
import { StepTeam } from "./StepTeam";
import { draftHasContent, emptyDraft, normalizeDomain, STEP_COUNT, STEP_LABELS, validateStep } from "./types";
import type { OnboardingDraft, OnboardingSession, OnboardingStep, OnboardingSubmit } from "./types";
import { clearDraft, readDraft, writeDraft } from "./storage";
import { FlowButton, FlowSkeleton, Note, Panel, StepProgress } from "./parts";

function stepAfter(step: OnboardingStep): OnboardingStep {
  if (step === 1) return 2;
  return 3;
}

function stepBefore(step: OnboardingStep): OnboardingStep {
  if (step === 3) return 2;
  return 1;
}

export function OnboardingFlow({
  session,
  onSubmit,
  doneHref = "/onboarding/done/",
}: {
  session: OnboardingSession;
  /** Optional server write. Undefined means: store the draft and go on. */
  onSubmit?: OnboardingSubmit;
  doneHref?: string;
}) {
  const hydrated = useHydrated();
  const router = useRouter();
  const [draft, setDraft] = useState<OnboardingDraft>(emptyDraft);
  const [ready, setReady] = useState(false);
  const [restored, setRestored] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!hydrated) return;
    const stored = readDraft(session.scope);
    if (stored) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- restoring a draft from localStorage
      setDraft(stored);
      setRestored(draftHasContent(stored));
    }
    setReady(true);
  }, [hydrated, session.scope]);

  // Persist after every change, and never overwrite a stored draft with a blank one.
  useEffect(() => {
    if (!ready || !draftHasContent(draft)) return;
    writeDraft(session.scope, draft);
  }, [ready, draft, session.scope]);

  const patch = (next: Partial<OnboardingDraft>) => {
    setDraft((prev) => ({ ...prev, ...next, updatedAt: new Date().toISOString() }));
  };

  function goNext() {
    const message = validateStep(draft.step, draft);
    if (message) {
      setError(message);
      return;
    }
    setError(null);
    setSubmitError(null);
    if (draft.step === STEP_COUNT) {
      void finish(false);
      return;
    }
    patch({ step: stepAfter(draft.step) });
  }

  function goBack() {
    if (draft.step === 1) return;
    setError(null);
    setSubmitError(null);
    patch({ step: stepBefore(draft.step) });
  }

  function startOver() {
    setReady(false);
    setRestored(false);
    setError(null);
    setSubmitError(null);
    setSubmitting(false);
    setDraft(emptyDraft());
    clearDraft(session.scope);
    setReady(true);
  }

  async function finish(skipInvites: boolean) {
    // Finishing from step 3 still checks the earlier answers: a cleared name or
    // a broken domain sends the guest back to the step that needs it.
    const firstProblem = validateStep(1, draft);
    if (firstProblem) {
      setError(firstProblem);
      patch({ step: 1 });
      return;
    }
    const secondProblem = validateStep(2, draft);
    if (secondProblem) {
      setError(secondProblem);
      patch({ step: 2 });
      return;
    }

    setSubmitting(true);
    setSubmitError(null);

    const now = new Date().toISOString();
    const completed: OnboardingDraft = {
      ...draft,
      orgName: draft.orgName.trim(),
      domain: draft.domainChoice === "own" ? normalizeDomain(draft.domain) : "",
      invites: skipInvites ? [] : draft.invites,
      step: 3,
      completedAt: now,
      updatedAt: now,
    };

    // Always keep the result locally: the arrival screen reads it from here.
    writeDraft(session.scope, completed);

    if (onSubmit) {
      let result: { ok: true } | { ok: false; error: string };
      try {
        result = await onSubmit(completed, session);
      } catch {
        result = { ok: false, error: "Could not reach the server. Your answers are saved in this browser, try again." };
      }
      if (!result.ok) {
        setSubmitError(result.error);
        setSubmitting(false);
        return;
      }
    }

    setDraft(completed);
    router.push(doneHref);
  }

  const step = draft.step;
  const domainError = error && step === 2 ? error : null;

  return (
    <div>
      <header>
        <Eyebrow>Get started</Eyebrow>
        <h1 className="mt-3 font-display text-3xl font-semibold tracking-tight text-balance sm:text-4xl">
          Set up your workspace
        </h1>
        <p className="mt-3 max-w-2xl text-lg text-muted">
          Three short steps: name the organization, choose how mail leaves it, then invite the people who will work
          in it. Nothing is published and nothing is sent while you decide.
        </p>
      </header>

      {ready ? <StepProgress current={step} total={STEP_COUNT} labels={STEP_LABELS} /> : <FlowSkeleton label="Loading your saved answers" />}

      {ready ? (
        <>
          {restored && draft.completedAt === null ? (
            <p className="mt-6 text-sm text-muted">
              Picked up where you left off in this browser.
            </p>
          ) : null}

          {draft.completedAt !== null ? (
            <div className="mt-6">
              <Note tone="success">
                Setup was already finished on this browser.{" "}
                <Link className="text-primary underline underline-offset-2" href={doneHref}>
                  Open the checklist
                </Link>{" "}
                to see what is done and what is left.
              </Note>
            </div>
          ) : null}

          <form
            className="mt-6"
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              goNext();
            }}
          >
            <Panel>
              {step === 1 ? (
                <StepOrganization
                  value={draft.orgName}
                  onChange={(orgName) => {
                    patch({ orgName });
                    if (error) setError(null);
                  }}
                  error={error}
                />
              ) : null}

              {step === 2 ? (
                <StepDomain
                  choice={draft.domainChoice}
                  domain={draft.domain}
                  onChoice={(domainChoice) => {
                    patch({ domainChoice });
                    setError(null);
                  }}
                  onDomain={(domain) => {
                    patch({ domain });
                    setError(null);
                  }}
                  error={domainError}
                />
              ) : null}

              {step === 3 ? (
                <StepTeam
                  invites={draft.invites}
                  onAdd={(email) => patch({ invites: [...draft.invites, email] })}
                  onRemove={(email) => patch({ invites: draft.invites.filter((value) => value !== email) })}
                />
              ) : null}
            </Panel>

            {submitError ? (
              <p className="mt-4 text-sm text-danger" role="alert">
                {submitError}
              </p>
            ) : null}

            <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex flex-wrap gap-2">
                <FlowButton variant="secondary" onClick={goBack} disabled={step === 1 || submitting}>
                  Back
                </FlowButton>
                <FlowButton variant="ghost" onClick={startOver} disabled={submitting}>
                  Start over
                </FlowButton>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                {step === 3 ? (
                  <FlowButton variant="secondary" onClick={() => void finish(true)} disabled={submitting}>
                    Skip for now
                  </FlowButton>
                ) : null}
                <FlowButton type="submit" disabled={submitting} aria-busy={submitting || undefined}>
                  {submitting ? "Saving" : step === 3 ? "Finish setup" : "Continue"}
                </FlowButton>
              </div>
            </div>

            <p className="mt-4 text-xs text-muted">
              Answers are saved in this browser as you type. No account is needed for that.
            </p>
          </form>
        </>
      ) : null}
    </div>
  );
}
