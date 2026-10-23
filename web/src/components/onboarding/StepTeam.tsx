"use client";

import { useId, useState } from "react";
import { isValidEmail, MAX_INVITES } from "./types";
import { EmptyState, Field, Note, TextInput } from "./parts";

export function StepTeam({
  invites,
  onAdd,
  onRemove,
}: {
  invites: string[];
  onAdd: (email: string) => void;
  onRemove: (email: string) => void;
}) {
  const id = useId();
  const [entry, setEntry] = useState("");
  const [error, setError] = useState<string | null>(null);
  const full = invites.length >= MAX_INVITES;

  function add() {
    const email = entry.trim().toLowerCase();
    if (email.length === 0) {
      setError("Type an address first.");
      return;
    }
    if (!isValidEmail(email)) {
      setError("Enter a valid email address like ada@example.com.");
      return;
    }
    if (invites.includes(email)) {
      setError("That address is already on the list.");
      return;
    }
    if (full) {
      setError(`Up to ${MAX_INVITES} invitations here. More can follow from the console.`);
      return;
    }
    setError(null);
    setEntry("");
    onAdd(email);
  }

  return (
    <div>
      <h2 className="font-display text-xl font-semibold tracking-tight">Invite your team</h2>
      <p className="mt-2 text-sm text-muted">
        Optional. Each teammate gets their own API keys with the permissions you pick, and shares this organization
        and its inboxes. You can add people later from the console.
      </p>

      <div className="mt-5 flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          <Field
            id={id}
            label="Teammate email"
            hint={full ? `The list is full at ${MAX_INVITES}.` : "Press Enter or use the button to add it."}
            error={error ?? undefined}
          >
            <TextInput
              id={id}
              value={entry}
              onChange={(event) => setEntry(event.target.value)}
              onKeyDown={(event) => {
                if (event.key !== "Enter") return;
                // The step sits inside the funnel form: keep Enter from submitting it.
                event.preventDefault();
                add();
              }}
              type="email"
              inputMode="email"
              autoComplete="email"
              autoCapitalize="none"
              spellCheck={false}
              disabled={full}
              placeholder="ada@example.com"
              aria-invalid={error ? true : undefined}
            />
          </Field>
        </div>
        <button
          type="button"
          onClick={add}
          disabled={full}
          className="inline-flex min-h-11 items-center justify-center rounded-lg border border-border bg-card px-4 text-sm font-medium transition-colors hover:border-fg/40 disabled:cursor-not-allowed disabled:opacity-50"
        >
          Add
        </button>
      </div>

      <div className="mt-5">
        {invites.length === 0 ? (
          <EmptyState title="No teammates added">
            The setup finishes without invitations. Skipping keeps the checklist identical.
          </EmptyState>
        ) : (
          <>
            <p className="text-xs text-muted">
              {invites.length} of {MAX_INVITES} invitations ready.
            </p>
            <ul className="mt-2 flex flex-wrap gap-2">
              {invites.map((email) => (
                <li
                  key={email}
                  className="flex items-center gap-1 rounded-lg border border-border bg-bg py-1 pr-1 pl-3 text-sm"
                >
                  <span className="font-mono break-all">{email}</span>
                  <button
                    type="button"
                    onClick={() => onRemove(email)}
                    aria-label={`Remove ${email}`}
                    className="inline-flex size-11 shrink-0 items-center justify-center rounded-md text-muted hover:bg-card hover:text-fg"
                  >
                    <svg viewBox="0 0 24 24" className="size-4" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8">
                      <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
                    </svg>
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>

      <div className="mt-5">
        <Note>
          Nothing is sent yet. Invitations are saved with this draft and go out once sign up and email delivery are
          connected, so no stranger receives a link today.
        </Note>
      </div>
    </div>
  );
}
