"use client";

// The persistent setup checklist. Ticks and the dismiss flag live in storage
// per session scope, so they survive a reload, a new tab and a return visit.
// Nothing here talks to the API: the list is a reminder, not a source of truth.

import { useEffect, useId, useState } from "react";
import Link from "next/link";
import { useHydrated } from "@/lib/use-hydrated";
import { emptyChecklist } from "./types";
import type { ChecklistItem, ChecklistItemId, ChecklistState } from "./types";
import { readChecklist, writeChecklist } from "./storage";
import { EmptyState, FlowButton, Note, Panel } from "./parts";

export function Checklist({ scope, items }: { scope: string; items: ChecklistItem[] }) {
  const base = useId();
  const hydrated = useHydrated();
  const [state, setState] = useState<ChecklistState>(emptyChecklist);
  const [ready, setReady] = useState(false);
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    if (!hydrated) return;
    setState(readChecklist(scope));
    setReady(true);
  }, [hydrated, scope]);

  useEffect(() => {
    if (!ready) return;
    writeChecklist(scope, state);
  }, [ready, scope, state]);

  const total = items.length;
  const done = items.filter((item) => state.done.includes(item.id)).length;
  const percent = total === 0 ? 0 : Math.round((done / total) * 100);
  const complete = total > 0 && done === total;

  function toggle(id: ChecklistItemId, checked: boolean) {
    setState((prev) => ({
      ...prev,
      done: checked ? [...prev.done.filter((value) => value !== id), id] : prev.done.filter((value) => value !== id),
    }));
    setAnnouncement(checked ? "Step marked done." : "Step reopened.");
  }

  function hide() {
    setState((prev) => ({ ...prev, dismissed: true }));
    setAnnouncement("Checklist hidden.");
  }

  function show() {
    setState((prev) => ({ ...prev, dismissed: false }));
    setAnnouncement("Checklist shown.");
  }

  if (!ready) {
    return (
      <Panel>
        <div role="status" className="h-4 w-40 animate-pulse rounded bg-border" />
        <div className="mt-3 h-2 w-full animate-pulse rounded-full bg-border" />
        <div className="mt-6 h-24 w-full animate-pulse rounded-lg bg-border" />
        <span className="sr-only">Loading the setup checklist</span>
      </Panel>
    );
  }

  if (state.dismissed) {
    return (
      <Panel>
        <EmptyState
          title="Checklist hidden"
          action={
            <FlowButton variant="secondary" onClick={show}>
              Show checklist
            </FlowButton>
          }
        >
          Your ticks are kept. Bring the list back whenever you want it.
        </EmptyState>
        <p aria-live="polite" className="sr-only">
          {announcement}
        </p>
      </Panel>
    );
  }

  return (
    <Panel>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-display text-xl font-semibold tracking-tight">Setup checklist</h2>
          <p className="mt-1 text-sm text-muted">
            {done} of {total} done. Ticks stay in this browser, no account needed.
          </p>
        </div>
        <button
          type="button"
          onClick={hide}
          aria-label="Hide checklist"
          className="inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-muted hover:bg-bg hover:text-fg"
        >
          <svg viewBox="0 0 24 24" className="size-5" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8">
            <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
          </svg>
        </button>
      </div>

      <div
        role="progressbar"
        aria-label="Setup checklist progress"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent}
        aria-valuetext={`${done} of ${total} steps done`}
        className="mt-4 flex gap-1"
      >
        {items.map((item) => (
          <span
            key={item.id}
            aria-hidden="true"
            className={`h-2 flex-1 rounded-full ${state.done.includes(item.id) ? "bg-success" : "bg-border"}`}
          />
        ))}
      </div>

      {complete ? (
        <div className="mt-4">
          <Note tone="success">All four steps are ticked. Hide the list when you no longer need it.</Note>
        </div>
      ) : null}

      <ul className="mt-5 list-none p-0">
        {items.map((item) => {
          const inputId = `${base}-${item.id}`;
          const checked = state.done.includes(item.id);
          return (
            <li key={item.id} className="border-t border-border first:border-t-0">
              <div className="flex flex-col py-1">
                {/* The label wraps the box and the title, so the tap target is 44px tall
                    even though the checkbox itself is 20px. */}
                <label htmlFor={inputId} className="flex min-h-11 cursor-pointer items-center gap-3">
                  <input
                    id={inputId}
                    type="checkbox"
                    checked={checked}
                    onChange={(event) => toggle(item.id, event.target.checked)}
                    className="size-5 shrink-0 accent-primary"
                  />
                  <span className={`text-sm font-medium ${checked ? "text-muted line-through" : ""}`}>
                    {item.title}
                  </span>
                </label>
                <p className="mt-1 text-sm text-muted sm:pl-8">{item.detail}</p>
                <Link
                  href={item.href}
                  className="mt-1 inline-flex min-h-11 items-center self-start text-sm font-medium text-primary underline underline-offset-2 sm:pl-8"
                >
                  {item.cta}
                </Link>
              </div>
            </li>
          );
        })}
      </ul>

      <p aria-live="polite" className="sr-only">
        {announcement}
      </p>
    </Panel>
  );
}
