"use client";

// Arrival screen: the welcome copy, the outcome of the three steps in one
// sentence each, and the persistent checklist. Everything is read from the
// draft in storage, so a reload or a visit tomorrow looks the same.

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Eyebrow } from "@/components/ui";
import { useHydrated } from "@/lib/use-hydrated";
import { checklistItems } from "./types";
import type { OnboardingDraft, OnboardingSession } from "./types";
import { clearOnboarding, readDraft } from "./storage";
import { Checklist } from "./Checklist";
import { EmptyState, FlowButton, FlowSkeleton, Note, Panel } from "./parts";

const NEXT_LINKS: { href: string; label: string; detail: string }[] = [
  { href: "/docs/quickstart/", label: "Quickstart", detail: "One inbox and one reply, in five minutes." },
  { href: "/console/", label: "Console", detail: "Inboxes, domains, keys and the audit log." },
  { href: "/docs/mcp/", label: "MCP guide", detail: "Add the inbox tools to Claude Code, Cursor or Codex." },
];

export function Arrival({ session }: { session: OnboardingSession }) {
  const hydrated = useHydrated();
  const router = useRouter();
  const [draft, setDraft] = useState<OnboardingDraft | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!hydrated) return;
    setDraft(readDraft(session.scope));
    setReady(true);
  }, [hydrated, session.scope]);

  const orgName = draft?.orgName.trim() ?? "";
  const domain = draft?.domainChoice === "own" ? draft.domain.trim() : "";
  const testAddress = draft?.domainChoice === "test";
  const invites = draft?.invites.length ?? 0;
  const items = checklistItems(draft);

  function startOver() {
    clearOnboarding(session.scope);
    setDraft(null);
    router.push("/onboarding/");
  }

  return (
    <div>
      <header>
        <Eyebrow>Get started</Eyebrow>
        <h1 className="mt-3 font-display text-3xl font-semibold tracking-tight text-balance sm:text-4xl">
          {orgName ? `Welcome, ${orgName}` : "Welcome to multi.ai.inbox"}
        </h1>
        <p className="mt-3 max-w-2xl text-lg text-muted">
          {orgName
            ? `${orgName} is named and the first decisions are made. Four steps turn it into an agent with its own address, and the list keeps your ticks.`
            : "Four steps turn an empty workspace into an agent with its own address. The list keeps your ticks between visits."}
        </p>
      </header>

      <div className="mt-6 flex flex-col gap-3">
        {session.status === "anonymous" ? (
          <Note tone="warn">
            No account was created. Sign up is not open on this build, so these answers and the ticks below live in
            this browser only, and clearing site data removes them.
          </Note>
        ) : (
          <Note tone="success">
            {session.email ? `Signed in as ${session.email}.` : "Signed in."} The ticks below stay in this browser
            for now.
          </Note>
        )}

        {testAddress ? (
          <Note>
            You picked a free test address, so no DNS step is waiting. Inboxes on the shared test domain work
            without verification, and move to your own domain later.
          </Note>
        ) : null}

        {domain ? (
          <Note>
            You picked {domain}. Publish the four DNS records in the console and the checks run on their own every
            five minutes for seven days.
          </Note>
        ) : null}

        {invites > 0 ? (
          <Note>
            {invites} {invites === 1 ? "invitation is" : "invitations are"} queued with this draft. They go out when
            email delivery and accounts are connected, so nobody received a link yet.
          </Note>
        ) : null}

        {draft === null && ready ? (
          <Note>
            No draft on this browser yet.{" "}
            <Link className="text-primary underline underline-offset-2" href="/onboarding/">
              Walk through the three steps
            </Link>{" "}
            and the list below adapts to the domain you pick.
          </Note>
        ) : null}
      </div>

      {ready ? (
        <div className="mt-8 grid gap-6 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
          <Checklist scope={session.scope} items={items} />

          <aside className="flex flex-col gap-4">
            <Panel>
              <EmptyState
                title="Nothing here yet"
                action={
                  <Link
                    href="/console/"
                    className="inline-flex min-h-11 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-fg hover:opacity-90"
                  >
                    Open the console
                  </Link>
                }
              >
                This screen is the checklist, not the inbox. Inboxes, mail and keys show up in the console once an
                account is connected.
              </EmptyState>
            </Panel>

            <Panel>
              <h2 className="text-sm font-medium">Where to go next</h2>
              <ul className="mt-2 list-none p-0">
                {NEXT_LINKS.map((link) => (
                  <li key={link.href} className="border-t border-border first:border-t-0">
                    <Link href={link.href} className="flex min-h-11 flex-col justify-center py-2">
                      <span className="text-sm font-medium text-primary underline underline-offset-2">
                        {link.label}
                      </span>
                      <span className="text-xs text-muted">{link.detail}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            </Panel>
          </aside>
        </div>
      ) : (
        <FlowSkeleton label="Loading your workspace" />
      )}

      <footer className="mt-10 flex flex-wrap items-center gap-3 border-t border-border pt-6">
        <Link
          href="/onboarding/"
          className="inline-flex min-h-11 items-center text-sm font-medium text-primary underline underline-offset-2"
        >
          Back to the setup steps
        </Link>
        <FlowButton variant="ghost" onClick={startOver}>
          Start over
        </FlowButton>
      </footer>
    </div>
  );
}
